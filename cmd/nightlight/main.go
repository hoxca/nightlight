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

package main

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"runtime/debug"
	"strings"
	"time"
	nl "github.com/mlnoga/nightlight/internal"
	"github.com/pbnjay/memory"
)

const version = "0.2.5"

type Job struct {
	Id       int
	FileName string
	Image    *nl.FITSImage 
	Err      error
}

var totalMiBs=memory.TotalMemory()/1024/1024

var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to `file`")
var memprofile = flag.String("memprofile", "", "write memory profile to `file`")

var out  = flag.String("out", "out.fits", "save output to `file`")
var jpg  = flag.String("jpg", "%auto",  "save 8bit preview of output as JPEG to `file`. `%auto` replaces suffix of output file with .jpg")
var log  = flag.String("log", "%auto",    "save log output to `file`. `%auto` replaces suffix of output file with .log")
var pre  = flag.String("pre",  "",  "save pre-processed frames with given filename pattern, e.g. `pre%04d.fits`")
var stars= flag.String("stars","","save star detections with given filename pattern, e.g. `stars%04d.fits`")
var back = flag.String("back","","save extracted background with given filename pattern, e.g. `back%04d.fits`")
var post = flag.String("post", "",  "save post-processed frames with given filename pattern, e.g. `post%04d.fits`")
var batch= flag.String("batch", "", "save stacked batches with given filename pattern, e.g. `batch%04d.fits`")

var dark = flag.String("dark", "", "apply dark frame from `file`")
var flat = flag.String("flat", "", "apply flat frame from `file`")

var debayer = flag.String("debayer", "", "debayer the given channel, one of R, G, B or blank for no op")
var cfa     = flag.String("cfa", "RGGB", "color filter array type for debayering, one of RGGB, GRBG, GBRG, BGGR")

var binning= flag.Int64("binning", 0, "apply NxN binning, 0 or 1=no binning")

var bpSigLow  = flag.Float64("bpSigLow", 3.0,"low sigma for bad pixel removal as multiple of standard deviations")
var bpSigHigh = flag.Float64("bpSigHigh",5.0,"high sigma for bad pixel removal as multiple of standard deviations")

var starSig   = flag.Float64("starSig", 10.0,"sigma for star detection as multiple of standard deviations")
var starBpSig = flag.Float64("starBpSig",-1.0,"sigma for star detection bad pixel removal as multiple of standard deviations, -1: auto")
var starRadius= flag.Int64("starRadius", 16.0, "radius for star detection in pixels")

var backGrid  = flag.Int64("backGrid", 0, "automated background extraction: grid size in pixels, 0=off")
var backSigma = flag.Float64("backSigma", 1.5 ,"automated background extraction: sigma for detecting foreground objects")
var backClip  = flag.Int64("backClip", 0, "automated background extraction: clip the k brightest grid cells and replace with local median")

var usmSigma  = flag.Float64("usmSigma", 1, "unsharp masking sigma, ~1/3 radius")
var usmGain   = flag.Float64("usmGain", 0, "unsharp masking gain, 0=no op")
var usmThresh = flag.Float64("usmThresh", 1, "unsharp masking threshold, in standard deviations above background")

var align     = flag.Int64("align",1,"1=align frames, 0=do not align")
var alignK    = flag.Int64("alignK",20,"use triangles fromed from K brightest stars for initial alignment")
var alignT    = flag.Float64("alignT",1.0,"skip frames if alignment to reference frame has residual greater than this")

var lsEst     = flag.Int64("lsEst",3,"location and scale estimators 0=mean/stddev, 1=median/MAD, 2=IKSS, 3=iterative sigma-clipped sampled median and sampled Qn (standard)")
var normRange = flag.Int64("normRange",0,"normalize range: 1=normalize to [0,1], 0=do not normalize")
var normHist  = flag.Int64("normHist",3,"normalize histogram: 0=do not normalize, 1=location and scale, 2=black point shift for RGB align, 3=auto")

var stMode    = flag.Int64("stMode", 5, "stacking mode. 0=median, 1=mean, 2=sigma clip, 3=winsorized sigma clip, 4=linear fit, 5=auto")
var stClipPercLow = flag.Float64("stClipPercLow", 0.5,"set desired low clipping percentage for stacking, 0=ignore (overrides sigmas)")
var stClipPercHigh= flag.Float64("stClipPercHigh",0.5,"set desired high clipping percentage for stacking, 0=ignore (overrides sigmas)")
var stSigLow  = flag.Float64("stSigLow", -1,"low sigma for stacking as multiple of standard deviations, -1: use clipping percentage to find")
var stSigHigh = flag.Float64("stSigHigh",-1,"high sigma for stacking as multiple of standard deviations, -1: use clipping percentage to find")
var stWeight  = flag.Int64("stWeight", 0, "weights for stacking. 0=unweighted (default), 1=by exposure, 2=by inverse noise")
var stMemory  = flag.Int64("stMemory", int64((totalMiBs*7)/10), "total MiB of memory to use for stacking, default=0.7x physical memory")

var neutSigmaLow  = flag.Float64("neutSigmaLow", -1, "neutralize background color below this threshold, <0 = no op")
var neutSigmaHigh = flag.Float64("neutSigmaHigh", -1, "keep background color above this threshold, interpolate in between, <0 = no op")

var chromaGamma=flag.Float64("chromaGamma", 1.0, "scale LCH chroma curve by given gamma for luminances n sigma above background, 1.0=no op")
var chromaSigma=flag.Float64("chromaSigma", 1.0, "only scale and add to LCH chroma for luminances n sigma above background")

var chromaFrom= flag.Float64("chromaFrom", 295, "scale LCH chroma for hues in [from,to] by given factor, e.g. 295 to desaturate violet stars")
var chromaTo  = flag.Float64("chromaTo", 40, "scale LCH chroma for hues in [from,to] by given factor, e.g. 40 to desaturate violet stars")
var chromaBy  = flag.Float64("chromaBy", 1, "scale LCH chroma for hues in [from,to] by given factor, e.g. -1 to desaturate violet stars")

var rotFrom   = flag.Float64("rotFrom", 100, "rotate LCH color angles in [from,to] by given offset, e.g. 100 to aid Hubble palette for S2HaO3")
var rotTo     = flag.Float64("rotTo", 190, "rotate LCH color angles in [from,to] by given offset, e.g. 190 to aid Hubble palette for S2HaO3")
var rotBy     = flag.Float64("rotBy", 0, "rotate LCH color angles in [from,to] by given offset, e.g. -30 to aid Hubble palette for S2HaO3")

var scnr      = flag.Float64("scnr",0,"apply SCNR in [0,1] to green channel, e.g. 0.5 for tricolor with S2HaO3 and 0.1 for bicolor HaO3O3")

var autoLoc   = flag.Float64("autoLoc", 10, "histogram peak location in %% to target with automatic curves adjustment, 0=don't")
var autoScale = flag.Float64("autoScale", 0.4, "histogram peak scale in %% to target with automatic curves adjustment, 0=don't")

var midtone   = flag.Float64("midtone", 0, "midtone value in multiples of standard deviation; 0=no op")
var midBlack  = flag.Float64("midBlack", 2, "midtone black in multiples of standard deviation below background location")

var gamma     = flag.Float64("gamma", 1, "apply output gamma, 1: keep linear light data")
var ppGamma   = flag.Float64("ppGamma", 1, "apply post-peak gamma, scales curve from location+scale...ppLimit, 1: keep linear light data")
var ppSigma   = flag.Float64("ppSigma", 1, "apply post-peak gamma this amount of scales from the peak (to avoid scaling background noise)")

var scaleBlack= flag.Float64("scaleBlack", 0, "move black point so histogram peak location is given value in %%, 0=don't")

var darkF *nl.FITSImage=nil
var flatF *nl.FITSImage=nil

var lights   =[]*nl.FITSImage{}

func main() {
	debug.SetGCPercent(10)
	start:=time.Now()
	flag.Usage=func(){
 	    nl.LogPrintf(`Nightlight Copyright (c) 2020 Markus L. Noga
This program comes with ABSOLUTELY NO WARRANTY.
This is free software, and you are welcome to redistribute it under certain conditions.
Refer to https://www.gnu.org/licenses/gpl-3.0.en.html for details.

Usage: %s [-flag value] (stats|stack|rgb|argb|lrgb|legal) (img0.fits ... imgn.fits)

Commands:
  stats   Show input image statistics
  stack   Stack input images
  rgb     Combine color channels. Inputs are treated as r, g and b channel in that order
  argb    Combine color channels and align with luminance. Inputs are treated as l, r, g and b channels
  lrgb    Combine color channels and combine with luminance. Inputs are treated as l, r, g and b channels
  serve   Serve HTTP for interactive mode
  legal   Show license and attribution information
  version Show version information

Flags:
`, os.Args[0])
	    flag.PrintDefaults()
	}
	flag.Parse()

	// Initialize logging to file in addition to stdout, if selected
	if *log=="%auto" {
		if *out!="" {
			*log=strings.TrimSuffix(*out, filepath.Ext(*out))+".log"			
		} else {
			*log=""
		}
	}
	if *log!="" { 
		err:=nl.LogAlsoToFile(*log)
		if err!=nil { nl.LogFatalf("Unable to open logfile '%s'\n", *log) }
	}

	// Also auto-select JPEG output target
	if *jpg=="%auto" {
		if *out!="" {
			*jpg=strings.TrimSuffix(*out, filepath.Ext(*out))+".jpg"			
		} else {
			*jpg=""
		}
	}

	// Enable CPU profiling if flagged
    if *cpuprofile != "" {
        f, err := os.Create(*cpuprofile)
        if err != nil {
            nl.LogFatal("Could not create CPU profile: ", err)
        }
        defer f.Close()
        if err := pprof.StartCPUProfile(f); err != nil {
            nl.LogFatal("Could not start CPU profile: ", err)
        }
      defer pprof.StopCPUProfile()
    }

    // Set parameters from command line flags
	preP  :=ParseCmdLinePreProcessParams()
	postP :=ParseCmdLinePostProcessParams()
	stackP:=ParseCmdLineStackParams()
	cP    :=ParseCmdLineColorParams()
	tcP   :=ParseCmdLineToneCurveParams()

    // Check if command present
    args:=flag.Args()
    if len(args)<1 {
    	flag.Usage()
    	return
    }

	var fileNames []string
    if args[0]=="stats" || args[0]=="stack" || args[0]=="rgb" || args[0]=="argb" || args[0]=="lrgb" {
    	// Determine filenames to process
		fileNames=nl.GlobFilenameWildcards(args[1:])
		if fileNames==nil || len(fileNames)==0 {
			nl.LogFatal("Error: no input files")
		}
		nl.LogPrintf("Found %d frames:\n", len(fileNames))
		for i, fileName :=range fileNames {
			nl.LogPrintf("%d:%s\n",i, fileName)
		}

		// Determine location and scale estimator
	    nl.LogPrintf("Using location and scale estimator %d\n", *lsEst)
		nl.LSEstimator=nl.LSEstimatorMode(*lsEst)
	}

    switch args[0] {
    case "stats":
    	nl.CmdStats(fileNames, preP)
    case "stack":
    	nl.CmdStack(fileNames, preP, postP, stackP)
    case "rgb":
    	nl.CmdRgb(fileNames, preP, postP, cP, tcP, *out, *jpg)
    case "argb":
    	nl.CmdLrgb(fileNames, false, preP, postP, cP, tcP, *out, *jpg)
    case "lrgb":
    	nl.CmdLrgb(fileNames, true,  preP, postP, cP, tcP, *out, *jpg)
    case "legal":
    	nl.CmdLegal()
    case "version":
    	nl.LogPrintf("Version %s\n", version)
    case "help", "?":
    	flag.Usage()
    default:
    	nl.LogPrintf("Unknown command '%s'\n\n", args[0])
    	flag.Usage()
    	return 
    }

	now:=time.Now()
	elapsed:=now.Sub(start)
	nl.LogPrintf("\nDone after %v\n", elapsed)

	// Store memory profile if flagged
    if *memprofile != "" {
        f, err := os.Create(*memprofile)
        if err != nil {
            nl.LogFatal("Could not create memory profile: ", err)
        }
        defer f.Close()
        runtime.GC() // get up-to-date statistics
        if err := pprof.Lookup("allocs").WriteTo(f,0); err != nil {
            nl.LogFatal("Could not write allocation profile: ", err)
        }
    }
    nl.LogSync()
}


// Parses command line flags and fills preprocessing parameters 
func ParseCmdLinePreProcessParams() (p *nl.PreProcessParams) {
	return &nl.PreProcessParams{
		Dark: 		 *dark,
		Flat: 	     *flat,
		Debayer:     *debayer,
		CFA:         *cfa,
		Binning:     int(*binning),
		NormRange:   int(*normRange),
		NormHist:    int(*normHist),
		BpSigLow:    float32(*bpSigLow),
		BpSigHigh:   float32(*bpSigHigh),
		StarSig:     float32(*starSig),
		StarBpSig:   float32(*starBpSig),
		StarRadius:  int(*starRadius),
		StarPattern: *stars,
		BackGrid:    int(*backGrid),
		BackSigma:   float32(*backSigma),
		BackClip:    int(*backClip),
		BackPattern: *back,
		PrePattern:  *pre,
	}
}


// Parses command line flags and fills postprocessing parameters 
func ParseCmdLinePostProcessParams() (p *nl.PostProcessParams) {
	return &nl.PostProcessParams{
		Align:		 int32(*align),
		AlignK:		 int32(*alignK),
		AlignThresh: float32(*alignT),
		NormHist:	 nl.HistoNormMode(*normHist),
		OobMode:	 nl.OobModeNaN,
		UsmSigma:	 float32(*usmSigma),
		UsmGain: 	 float32(*usmGain),
		UsmThresh:	 float32(*usmThresh),
		PostPattern: string(*post),
	}
}


// Parses command line flags and fills stacking parameters 
func ParseCmdLineStackParams() (p *nl.StackParams) {
	return &nl.StackParams{
		Memory:       *stMemory,
		Mode:		  nl.StackMode(*stMode),
		Weighted:     int32(*stWeight),
		SigmaLow:     float32(*stSigLow),
		SigmaHigh:	  float32(*stSigHigh),
		ClipPercLow:  float32(*stClipPercLow),
		ClipPercHigh: float32(*stClipPercHigh),
		BatchPattern: *batch,
		OutName:      *out,
	}
}


// Parses command line flags and fills color enhancement parameters, after RGB combination 
func ParseCmdLineColorParams() (p *nl.ColorParams) {
	return &nl.ColorParams{
		NeutSigmaLow:  float32(*neutSigmaLow),
		NeutSigmaHigh: float32(*neutSigmaHigh),
		ChromaGamma:   float32(*chromaGamma),
		ChromaSigma:   float32(*chromaSigma),
		ChromaBy:      float32(*chromaBy),
		ChromaFrom:    float32(*chromaFrom),
		ChromaTo:      float32(*chromaTo),
	    RotBy:         float32(*rotBy),
	    RotFrom:       float32(*rotFrom),
	    RotTo:         float32(*rotTo),
	    Scnr:          float32(*scnr),
	}
}


// Parses command line flags and fills tone curve enhancement parameters, after color enhancement
func ParseCmdLineToneCurveParams() (p *nl.ToneCurveParams) {
	return &nl.ToneCurveParams{
	    AutoLoc:       float32(*autoLoc),
	    AutoScale:     float32(*autoScale),
	    Midtone:       float32(*midtone),
	    MidBlack:      float32(*midBlack),
	    Gamma:         float32(*gamma),
	    PpGamma:       float32(*ppGamma),
	    PpSigma:       float32(*ppSigma),
	    ScaleBlack:    float32(*scaleBlack),
	}
}
