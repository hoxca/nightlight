// Copyright (C) 2020 Markus L. Noga
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.


package internal

import (
	"errors"
	"fmt"
)


// Load dark frame from FITS file
func LoadDark(dark string) *FITSImage {
	darkF:=NewFITSImage()
	darkF.ID=-1
	err:=darkF.ReadFile(dark)
	if err!=nil { panic(err) }
	darkF.Stats=CalcBasicStats(darkF.Data)
	darkF.Stats.Noise=EstimateNoise(darkF.Data, darkF.Naxisn[0])
	LogPrintf("Dark %s stats: %v\n", dark, darkF.Stats)

	if darkF.Stats.StdDev<1e-8 {
		LogPrintf("Warnining: dark file may be degenerate\n")
	}
	return &darkF
}


// Load flat frame from FITS file
func LoadFlat(flat string) *FITSImage {
	flatF:=NewFITSImage()
	flatF.ID=-2
	err:=flatF.ReadFile(flat)
	if err!=nil { panic(err) }
	flatF.Stats=CalcBasicStats(flatF.Data)
	flatF.Stats.Noise=EstimateNoise(flatF.Data, flatF.Naxisn[0])
	LogPrintf("Flat %s stats: %v\n", flat, flatF.Stats)

	if (flatF.Stats.Min<=0 && flatF.Stats.Max>=0) || flatF.Stats.StdDev<1e-8 {
		LogPrintf("Warnining: flat file may be degenerate\n")
	}
	return &flatF
}


// Preprocess all light frames with given global settings, limiting concurrency to the number of available CPUs
func PreProcessLights(ids []int, fileNames []string, darkF, flatF *FITSImage, debayer, cfa string, binning, normRange int32, bpSigLow, bpSigHigh, starSig, starBpSig float32, starRadius int32, starsShow string, backGrid int32, backPattern, preprocessedPattern string, imageLevelParallelism int32) (lights []*FITSImage) {
	//LogPrintf("CSV Id,%s\n", (&BasicStats{}).ToCSVHeader())

	lights =make([]*FITSImage, len(fileNames))
	sem   :=make(chan bool, imageLevelParallelism)
	for i, fileName := range(fileNames) {
		id:=ids[i]
		sem <- true 
		go func(i int, id int, fileName string) {
			defer func() { <-sem }()
			lightP, err:=PreProcessLight(id, fileName, darkF, flatF, debayer, cfa, binning, normRange, bpSigLow, bpSigHigh, starSig, starBpSig, starRadius, backGrid, backPattern)
			if err!=nil {
				LogPrintf("%d: Error: %s\n", id, err.Error())
			} else {
				lights[i]=lightP
				if preprocessedPattern!="" {
					err=lightP.WriteFile(fmt.Sprintf(preprocessedPattern, id))
					if err!=nil { LogFatalf("Error writing file: %s\n", err) }
				}
				if starsShow!="" {
					stars:=ShowStars(lightP, 2.0)
					stars.WriteFile(fmt.Sprintf(starsShow, id))
					if err!=nil { LogFatalf("Error writing file: %s\n", err) }
				}
			}
		}(i, id, fileName)
	}
	for i:=0; i<cap(sem); i++ {  // wait for goroutines to finish
		sem <- true
	}
	return lights	
}

// Preprocess a single light frame with given settings.
// Pre-processing includes loading, basic statistics, dark subtraction, flat division, 
// bad pixel removal, star detection and HFR calculation.
func PreProcessLight(id int, fileName string, darkF, flatF *FITSImage, debayer, cfa string, binning, normRange int32, bpSigLow, bpSigHigh, 
	starSig, starBpSig float32, starRadius int32, backGrid int32, backPattern string) (lightP *FITSImage, err error) {
	// Load light frame
	light:=NewFITSImage()
	light.ID=id
	err=light.ReadFile(fileName)
	if err!=nil { return nil, err }

	//light.Stats=aim.CalcBasicStats(light.Data)
	//LogPrintf("%d: Light %v %d bpp, %v\n", id, light.Naxisn, light.Bitpix, light.Stats)

	// apply dark frame if available
	if darkF!=nil && darkF.Pixels>0 {
		if !EqualInt32Slice(darkF.Naxisn, light.Naxisn) {
			return nil, errors.New("light size differs from dark size")
		}
		Subtract(light.Data, light.Data, darkF.Data)
	}

	// apply flat frame if available
	if flatF!=nil && flatF.Pixels>0 {
		if !EqualInt32Slice(flatF.Naxisn, light.Naxisn) {
			return nil, errors.New("light size differs from flat size")
		}
		Divide(light.Data, light.Data, flatF.Data, flatF.Stats.Mean)
	}

	// debayer color filter array data if desired
	if debayer!="" {
		light.Data, light.Naxisn[0], err=DebayerBilinear(light.Data, light.Naxisn[0], debayer, cfa)
		if err!=nil { return nil, err }
		light.Pixels=int32(len(light.Data))
		light.Naxisn[1]=light.Pixels/light.Naxisn[0]
		LogPrintf("%d: Debayered channel %s from cfa %s, new size %dx%d\n", id, debayer, cfa, light.Naxisn[0], light.Naxisn[1])
	}

	// remove bad pixels if flagged
	// FIXME: wont work well on debayered data, need to rethink this!
	var medianDiffStats *BasicStats
	if bpSigLow!=0 && bpSigHigh!=0 {
		mask:=CreateMask(light.Naxisn[0], 1.5)
		var bpm []int32
		bpm, medianDiffStats=BadPixelMap(light.Data, light.Naxisn[0], mask, bpSigLow, bpSigHigh)
		MedianFilterSparse(light.Data, bpm, mask)
		LogPrintf("%d: Removed %d bad pixels (%.2f%%) with sigma low=%.2f high=%.2f\n", 
			id, len(bpm), 100.0*float32(len(bpm))/float32(light.Pixels), bpSigLow, bpSigHigh)
		bpm=nil
	}

	// apply binning if desired
	if binning>1 {
		binned:=BinNxN(&light, binning)
 		light=binned
	}

	// calculate stats and find stars
	light.Stats, err=CalcExtendedStats(light.Data, light.Naxisn[0])
	if err!=nil { return nil, err }
	light.Stars, _, light.HFR=FindStars(light.Data, light.Naxisn[0], light.Stats.Location, light.Stats.Scale, starSig, starBpSig, starRadius, medianDiffStats)
	LogPrintf("%d: Stars %d HFR %.3g %v\n", id, len(light.Stars), light.HFR, light.Stats)
	//LogPrintf("CSV %d,%s\n", id, light.Stats.ToCSVLine())

	// automatic background extraction, if desired
	if backGrid>0 {
		LogPrintf("%d: Automatic background extraction with grid size %d\n", id, backGrid)
		stars:=ShowStars(&light, 3.0)
		bg:=NewBackground(light.Data, stars.Data, light.Naxisn[0], backGrid)
		if backPattern=="" {
			bg.Subtract(light.Data)
		} else { 
			bgImage:=bg.Render()
			bgFits:=FITSImage{
				Header:NewFITSHeader(),
				Bitpix:-32,
				Bzero :0,
				Naxisn:light.Naxisn,
				Pixels:light.Pixels,
				Data  :bgImage,
			}
			err=bgFits.WriteFile(fmt.Sprintf("back%02d.fits", id))
			if err!=nil { LogFatalf("Error writing file: %s\n", err) }
			Subtract(light.Data, light.Data, bgImage)
			bgFits.Data, bgImage=nil, nil
		}

		// re-do stats and star detection
		light.Stats, err=CalcExtendedStats(light.Data, light.Naxisn[0])
		if err!=nil { return nil, err }
		light.Stars, _, light.HFR=FindStars(light.Data, light.Naxisn[0], light.Stats.Location, light.Stats.Scale, starSig, starBpSig, starRadius, medianDiffStats)
		LogPrintf("%d: Stars %d HFR %.3g %v\n", id, len(light.Stars), light.HFR, light.Stats)
	}

	// Normalize value range if desired
	if normRange>0 {
		if light.Stats.Min==light.Stats.Max {
			LogPrintf("%d: Warning: Image is of uniform intensity %.4g, skipping normalization\n", id, light.Stats.Min)
		} else {
			LogPrintf("%d: Normalizing from [%.4g,%.4g] to [0,1]\n", id, light.Stats.Min, light.Stats.Max)
	    	light.Normalize()
			light.Stats, err=CalcExtendedStats(light.Data, light.Naxisn[0])
			if err!=nil { return nil, err }
		}
	}

	return &light, nil
}


// Select reference frame by maximizing the number of stars divided by HFR
func SelectReferenceFrame(lights []*FITSImage) (refFrame *FITSImage, refScore float32) {
	refFrame, refScore=(*FITSImage)(nil), -1
	for _, lightP:=range lights {
		if lightP==nil { continue }
		score:=float32(len(lightP.Stars))/lightP.HFR
		if len(lightP.Stars)==0 || lightP.HFR==0 { score=0 }
		if score>refScore {
			refFrame, refScore = lightP, score
		}
	}	
	return refFrame, refScore
}

