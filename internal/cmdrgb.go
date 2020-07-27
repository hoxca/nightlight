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
	"runtime"
)


// Perform RGB combination command
func CmdRgb(fileNames []string, preP *PreProcessParams, postP *PostProcessParams, cP *ColorParams, tcP *ToneCurveParams, outName, jpgName string) {
	// Set default parameters for this command
	if preP.NormHist==HNMAuto { preP.NormHist=HNMNone }
	if preP.StarBpSig<0 { preP.StarBpSig=0 }  // inputs are typically stacked and have undergone noise removal

	if len(fileNames)!=3 {
		LogFatal("Need exactly three input files to perform a RGB combination")
	}
	ids:=[]int{0,1,2}

	// Read files and detect stars
	imageLevelParallelism:=int32(runtime.GOMAXPROCS(0))
	if imageLevelParallelism>3 { imageLevelParallelism=3 }
	LogPrintf("\nReading color channels and detecting stars:\n")
	preP.NormRange, preP.BpSigLow, preP.BpSigHigh = 1, 0, 0
	lights:=PreProcessLights(ids, fileNames, nil, nil, preP, imageLevelParallelism)

	// Pick reference frame
	var refFrame *FITSImage
	var refFrameScore float32

	if postP.Align!=0 || preP.NormHist!=0 {
		refFrame, refFrameScore=SelectReferenceFrame(lights)
		if refFrame==nil { panic("Reference channel for alignment not found.") }
		LogPrintf("Using channel %d with score %.4g as reference for alignment and normalization.\n\n", refFrame.ID, refFrameScore)
	}

	// Post-process all channels (align, normalize)
	postP.OobMode=OobModeOwnLocation
	LogPrintf("Postprocessing %d channels with %s:\n", len(lights), postP)
	numErrors:=PostProcessLights(refFrame, refFrame, lights, postP, imageLevelParallelism)
    if numErrors>0 { LogFatal("Need aligned RGB frames to proceed") }

	// Combine RGB channels
	LogPrintf("\nCombining color channels...\n")
	rgb:=CombineRGB(lights, refFrame)

	PostProcessAndSaveRgbComposite(&rgb, nil, cP, tcP, outName, jpgName)
	rgb.Data=nil
}


// Perform LRGB combination command
func CmdLrgb(fileNames []string, applyLuminance bool, preP *PreProcessParams, postP *PostProcessParams, cP *ColorParams, tcP *ToneCurveParams, outName, jpgName string) {
	// Set default parameters for this command
	if preP.NormHist==HNMAuto { preP.NormHist=HNMNone }
	if preP.StarBpSig<0 { preP.StarBpSig=0 }  // inputs are typically stacked and have undergone noise removal

	if len(fileNames)!=4 {
		LogFatal("Need exactly four input files to perform a LRGB combination")
	}
	ids:=[]int{0,1,2,3}

	// Read files and detect stars
	imageLevelParallelism:=int32(runtime.GOMAXPROCS(0))
	if imageLevelParallelism>4 { imageLevelParallelism=4 }
	LogPrintf("\nReading color channels and detecting stars:\n")
	preP.NormRange, preP.BpSigLow, preP.BpSigHigh = 1, 0, 0
	lights:=PreProcessLights(ids, fileNames, nil, nil, preP, imageLevelParallelism)

	var refFrame, histoRef *FITSImage
	if postP.Align!=0 {
		// Always use luminance as reference frame
		refFrame=lights[0]
		LogPrintf("Using luminance channel %d as reference for alignment.\n", refFrame.ID)
	}

	if preP.NormHist!=0 {
		// Normalize to [0,1]
		histoRef=lights[1]
		minLoc:=float32(histoRef.Stats.Location)
	    for id, light:=range(lights) {
	    	if id>0 && light.Stats.Location<minLoc { 
	    		minLoc=light.Stats.Location 
	    		histoRef=light
	    	}
	    }
		LogPrintf("Using color channel %d as reference for RGB peak normalization to %.4g...\n\n", histoRef.ID, histoRef.Stats.Location)
	}

	// Align images if selected
	postP.OobMode=OobModeOwnLocation
	LogPrintf("Postprocessing %d channels with %s:\n", len(lights), postP)
	numErrors:=PostProcessLights(refFrame, histoRef, lights, postP, imageLevelParallelism)
    if numErrors>0 { LogFatal("Need aligned RGB frames to proceed") }

	// Combine RGB channels
	LogPrintf("\nCombining color channels...\n")
	rgb:=CombineRGB(lights[1:], lights[0])

	if applyLuminance {
		PostProcessAndSaveRgbComposite(&rgb, lights[0], cP, tcP, outName, jpgName)
	} else {
		PostProcessAndSaveRgbComposite(&rgb, nil,       cP, tcP, outName, jpgName)
	}
	rgb.Data=nil
}
