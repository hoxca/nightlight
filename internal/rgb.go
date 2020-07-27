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
	"fmt"
	"math"
)


// Parameters for RGB combination and enhancement, after stacking
type ColorParams struct {
	NeutSigmaLow  float32
	NeutSigmaHigh float32
	ChromaGamma   float32
	ChromaSigma   float32
	ChromaBy      float32
	ChromaFrom    float32
	ChromaTo      float32
    RotBy         float32
    RotFrom       float32
    RotTo         float32
    Scnr          float32
}

// Print parameters for stacking subexposures
func (p *ColorParams) String() string {
	return fmt.Sprintf("neutSigmaLow %.2f neutSigmaHigh %.2f chromaGamma %.2f chromaSigma %.2f "+
		               "chromaBy %.2f chromaFrom %.2f chromaTo %.2f rotBy %.2f rotFrom %.2f rotTo %.2f "+
		               " scnr %.2f",
					   p.NeutSigmaLow, p.NeutSigmaHigh, p.ChromaGamma, p.ChromaSigma, 
					   p.ChromaBy, p.ChromaFrom, p.ChromaTo, p.RotBy, p.RotFrom, p.RotTo, p.Scnr)
}

type ToneCurveParams struct {
    AutoLoc       float32
    AutoScale     float32
    Midtone       float32
    MidBlack      float32
    Gamma         float32
    PpGamma       float32
    PpSigma       float32
    ScaleBlack    float32
}

// Print parameters for stacking subexposures
func (p *ToneCurveParams) String() string {
	return fmt.Sprintf("autoLoc %.2f autoScale %.2f midtone %.2f midBlack %.2f gamma %.2f "+
		               "ppGamma %.2f ppSigma %.2f scaleBlack %.2f",
					   p.AutoLoc, p.AutoScale, p.Midtone, p.MidBlack, p.Gamma, 
					   p.PpGamma, p.PpSigma, p.ScaleBlack)
}


func PostProcessAndSaveRgbComposite(rgb *FITSImage, lum *FITSImage, cP *ColorParams, tcP *ToneCurveParams, outName, jpgName string) {
	AutoBalanceColors(rgb)
	CombineLrgb(rgb, lum)
	EnhanceColors(rgb, cP)
	EnhanceToneCurve(rgb, tcP)

	// Write outputs
	LogPrintf("Writing FITS to %s ...\n", outName)
	err:=rgb.WriteFile(outName)
	if err!=nil { LogFatalf("Error writing file: %s\n", err) }
	if jpgName!="" {
		LogPrintf("Writing JPG to %s ...\n", jpgName)
		rgb.WriteJPGToFile(jpgName, 95)
		if err!=nil { LogFatalf("Error writing file: %s\n", err) }
	}
}
	

// Automatically balance colors with multiple iterations of SetBlackWhitePoints, producing log output
func AutoBalanceColors(rgb *FITSImage) {
	if len(rgb.Stars)==0 {
		LogPrintln("Skipping black and white point adjustment as zero stars have been detected")
	} else {
		LogPrintln("Setting black point so histogram peaks align and white point so median star color becomes neutral...")
		for i:=0; i<3; i++ {
			err:=rgb.SetBlackWhitePoints()
			if err!=nil { LogFatal(err) }
		}
	}
}


func CombineLrgb(rgb *FITSImage, lum *FITSImage) {
	// Apply LRGB combination in linear CIE xyY color space
	if lum!=nil {
		LogPrintln("Converting linear RGB to linear CIE xyY for LRGB combination")
	    rgb.ToXyy()

		LogPrintln("Applying luminance to Y channel...")
		rgb.ApplyLuminanceToCIExyY(lum)

		LogPrintln("Converting linear CIE xyY to linear RGB")
		rgb.XyyToRGB()
	}
}


func EnhanceColors(rgb *FITSImage, cP *ColorParams) {
	// Apply color corrections in non-linear modified CIE L*C*H space, i.e. HSL
	if (cP.NeutSigmaLow>=0 && cP.NeutSigmaHigh>=0) || (cP.ChromaGamma!=1) || (cP.ChromaBy!=0) || (cP.RotBy!=0) || (cP.Scnr!=0) {
		LogPrintln("Converting image to nonlinear modified CIE L*C*H space, i.e. HSL...")
		rgb.RGBToCIEHSL()

	    if cP.NeutSigmaLow>=0 && cP.NeutSigmaHigh>=0 {
			LogPrintf("Neutralizing background values below %.4g sigma, keeping color above %.4g sigma\n", cP.NeutSigmaLow, cP.NeutSigmaHigh)    	

			loc, scale, err:=HCLLumLocScale(rgb.Data, rgb.Naxisn[0])
			if err!=nil { LogFatal(err) }
			low :=loc + scale*cP.NeutSigmaLow
			high:=loc + scale*cP.NeutSigmaHigh
			LogPrintf("Location %.2f%%, scale %.2f%%, low %.2f%% high %.2f%%\n", loc*100, scale*100, low*100, high*100)

			rgb.NeutralizeBackground(low, high)		
	    }

	    if cP.ChromaGamma!=1 {
	    	LogPrintf("Applying gamma %.2f to saturation for values %.4g sigma above background...\n", cP.ChromaGamma, cP.ChromaSigma)

			// calculate basic image stats as a fast location and scale estimate
			loc, scale, err:=HCLLumLocScale(rgb.Data, rgb.Naxisn[0])
			if err!=nil { LogFatal(err) }
			threshold :=loc + scale*cP.ChromaSigma
			LogPrintf("Location %.2f%%, scale %.2f%%, threshold %.2f%%\n", loc*100, scale*100, threshold*100)

			rgb.AdjustChroma(cP.ChromaGamma, threshold)
	    }

	    if cP.ChromaBy!=1 {
	    	LogPrintf("Multiplying LCH chroma (saturation) by %.4g for hues in [%g,%g]...\n", cP.ChromaBy, cP.ChromaFrom, cP.ChromaTo)
			rgb.AdjustChromaForHues(cP.ChromaFrom, cP.ChromaTo, cP.ChromaBy)
	    }

	    if cP.RotBy!=0 {
	    	LogPrintf("Rotating LCH hue angles in [%g,%g] by %.4g...\n", cP.RotFrom, cP.RotTo, cP.RotBy)
			rgb.RotateColors(cP.RotFrom, cP.RotTo, cP.RotBy)
	    }

	    if cP.Scnr!=0 {
	    	LogPrintf("Applying SCNR of %.4g ...\n", cP.Scnr)
			rgb.SCNR(cP.Scnr)
	    }

		LogPrintln("Converting nonlinear CIE HSL to linear RGB")
	    rgb.CIEHSLToRGB()
	}
}


func EnhanceToneCurve(rgb *FITSImage, tcP *ToneCurveParams) {
	// Apply luminance curves in linear CIE xyY color space
	if (tcP.AutoLoc!=0 && tcP.AutoScale!=0) || (tcP.Midtone!=0) || (tcP.Gamma!=1) || (tcP.PpGamma!=1) || (tcP.ScaleBlack!=0) {
		LogPrintln("Converting linear RGB to linear CIE xyY")
	    rgb.ToXyy()

		// Iteratively adjust gamma and shift back histogram peak
		if tcP.AutoLoc!=0 && tcP.AutoScale!=0 {
			targetLoc  :=tcP.AutoLoc/100.0    // range [0..1], while autoLoc is [0..100]
			targetScale:=tcP.AutoScale/100.0  // range [0..1], while autoScale is [0..100]
			LogPrintf("Automatic curves adjustment targeting location %.2f%% and scale %.2f%% ...\n", targetLoc*100, targetScale*100)

			for i:=0; ; i++ {
				if i==30 { 
					LogPrintf("Warning: did not converge after %d iterations\n",i)
					break
				}

				// calculate basic image stats as a fast location and scale estimate
				loc, scale, err:=HCLLumLocScale(rgb.Data, rgb.Naxisn[0])
				if err!=nil { LogFatal(err) }
				LogPrintf("Location %.2f%% and scale %.2f%%: ", loc*100, scale*100)

				if loc<=targetLoc*1.01 && scale<targetScale {
					idealGamma:=float32(math.Log((float64(targetLoc)/float64(targetScale))*float64(scale))/math.Log(float64(targetLoc)))
					if idealGamma>1.5 { idealGamma=1.5 }
					if idealGamma<=1.01 { 
						LogPrintf("done\n")
						break
					}

					LogPrintf("applying gamma %.3g\n", idealGamma)
					rgb.ApplyGammaToChannel(2, idealGamma)
				} else if loc>targetLoc*0.99 && scale<targetScale {
					LogPrintf("scaling black to move location to %.2f%%...\n", targetLoc*100)
					rgb.ShiftBlackToMoveChannel(2, loc, targetLoc)
				} else {
					LogPrintf("done\n")
					break
				}
			}
		}

	    // Optionally adjust midtones
	    if tcP.Midtone!=0 {
	    	LogPrintf("Applying midtone correction with midtone=%.2f%% x scale and black=location - %.2f%% x scale\n", tcP.Midtone, tcP.MidBlack)

			// calculate basic image stats as a fast location and scale estimate
			loc, scale, err:=HCLLumLocScale(rgb.Data, rgb.Naxisn[0])
			if err!=nil { LogFatal(err) }
			absMid:=tcP.Midtone*scale
			absBlack:=loc - tcP.MidBlack*scale
	    	LogPrintf("loc %.2f%% scale %.2f%% absMid %.2f%% absBlack %.2f%%\n", 100*loc, 100*scale, 100*absMid, 100*absBlack)
	    	rgb.ApplyMidtonesToChannel(2, absMid, absBlack)
	    }

		// Optionally adjust gamma 
		if tcP.Gamma!=1 {
			LogPrintf("Applying gamma %.3g\n", tcP.Gamma)
			rgb.ApplyGammaToChannel(2, tcP.Gamma)
		}

		// Optionally adjust gamma post peak
	    if tcP.PpGamma!=1 {
			loc, scale, err:=HCLLumLocScale(rgb.Data, rgb.Naxisn[0])
			if err!=nil { LogFatal(err) }

	    	from:=loc+tcP.PpSigma*scale
	    	to  :=float32(1.0)
	    	LogPrintf("Based on sigma=%.4g, boosting values in [%.2f%%, %.2f%%] with gamma %.4g...\n", tcP.PpSigma, from*100, to*100, tcP.PpGamma)
			rgb.ApplyPartialGammaToChannel(2, from, to, tcP.PpGamma)
	    }

		// Optionally scale histogram peak
	    if tcP.ScaleBlack!=0 {
	    	targetBlack:=tcP.ScaleBlack/100.0
			loc, scale, err:=HCLLumLocScale(rgb.Data, rgb.Naxisn[0])
			if err!=nil { LogFatal(err) }
			LogPrintf("Location %.2f%% and scale %.2f%%: ", loc*100, scale*100)

			if loc>targetBlack {
				LogPrintf("scaling black to move location to %.2f%%...\n", targetBlack*100.0)
				rgb.ShiftBlackToMoveChannel(2,loc, targetBlack)
			} else {
				LogPrintf("cannot move to location %.2f%% by scaling black\n", targetBlack*100.0)
			}
	    }

		LogPrintln("Converting linear CIE xyY to linear RGB")
		rgb.XyyToRGB()
	}
}
