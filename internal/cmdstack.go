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
	"runtime/debug"
)


// Perform stacking command. Updates stackP with sigma bounds if found.
func CmdStack(fileNames []string, preP *PreProcessParams, postP *PostProcessParams, stackP *StackParams) {
	// Set default parameters for this command
	if preP.NormHist==HNMAuto { preP.NormHist=HNMLocScale }
	if preP.StarBpSig<0 { preP.StarBpSig=5 } // default to noise elimination when working with individual subexposures

	// The stack of stacks
	var stack *FITSImage = nil
	var stackFrames int64 = 0
	var stackNoise  float32 = 0

	darkF, flatF, err:=LoadDarkAndFlat(preP.Dark, preP.Flat)
	if err!=nil { LogFatal(err) }

	// Split input into required number of randomized batches, given the permissible amount of memory
	numBatches, batchSize, overallIDs, overallFileNames, imageLevelParallelism:=PrepareBatches(fileNames, stackP.Memory, darkF, flatF)

	// Process each batch. The first batch sets the reference image, and if solving for sigLow/High also those. 
	// They are then reused in subsequent batches
	refFrame:=(*FITSImage)(nil)
	for b:=int64(0); b<numBatches; b++ {
		// Cut out relevant part of the overall input filenames
		batchStartOffset:= b   *batchSize
		batchEndOffset  :=(b+1)*batchSize
		if batchEndOffset>int64(len(fileNames)) { batchEndOffset=int64(len(fileNames)) }
		batchFrames     :=batchEndOffset-batchStartOffset
		ids      :=overallIDs      [batchStartOffset:batchEndOffset]
		fileNames:=overallFileNames[batchStartOffset:batchEndOffset]
		LogPrintf("\nStarting batch %d of %d with %d images: %v...\n", b, numBatches, len(ids), ids)

		// Stack the files in this batch 
		batch, avgNoise :=(*FITSImage)(nil), float32(0)
		batch, refFrame, avgNoise=stackBatch(ids, fileNames, darkF, flatF, refFrame, preP, postP, stackP, imageLevelParallelism)

		// Find stars in the newly stacked batch and report out on them
		batch.Stars, _, batch.HFR=FindStars(batch.Data, batch.Naxisn[0], batch.Stats.Location, batch.Stats.Scale, 
			preP.StarSig, preP.StarBpSig, int32(preP.StarRadius), nil)
		LogPrintf("Batch %d stack: Stars %d HFR %.2f Exposure %gs %v\n", b, len(batch.Stars), batch.HFR, batch.Exposure, batch.Stats)

		expectedNoise:=avgNoise/float32(math.Sqrt(float64(batchFrames)))
		LogPrintf("Batch %d expected noise %.4g from stacking %d frames with average noise %.4g\n",
					b, expectedNoise, int(batchFrames), avgNoise )

		// Save batch if desired
		if stackP.BatchPattern!="" {
			batchFileName:=fmt.Sprintf(stackP.BatchPattern, b)
			LogPrintf("Writing batch result to %s\n", batchFileName)
			err:=batch.WriteFile(batchFileName)
			if err!=nil { LogFatalf("Error writing file: %s\n", err) }
		}

		// Update stack of stacks
		if numBatches>1 {
			stack=StackIncremental(stack, batch, float32(batchFrames))
			stackFrames+=batchFrames
			stackNoise +=batch.Stats.Noise*float32(batchFrames)
		} else {
			stack=batch
		}

		// Free memory
		ids, fileNames, batch=nil, nil, nil
		debug.FreeOSMemory()
	}

	// Free more memory
	refFrame=nil  // all other primary frames already freed after stacking
	if darkF!=nil { darkF=nil }
	if flatF!=nil { flatF=nil }
	debug.FreeOSMemory()

	if numBatches>1 {
		// Finalize stack of stacks
		err:=StackIncrementalFinalize(stack, float32(stackFrames))
		if err!=nil { LogPrintf("Error calculating extended stats: %s\n", err) }

		// Find stars in newly stacked image and report out on them
		stack.Stars, _, stack.HFR=FindStars(stack.Data, stack.Naxisn[0], stack.Stats.Location, stack.Stats.Scale, 
			preP.StarSig, preP.StarBpSig, int32(preP.StarRadius), nil)
		LogPrintf("Overall stack: Stars %d HFR %.2f Exposure %gs %v\n", len(stack.Stars), stack.HFR, stack.Exposure, stack.Stats)

		avgNoise:=stackNoise/float32(stackFrames)
		expectedNoise:=avgNoise/float32(math.Sqrt(float64(numBatches)))
		LogPrintf("Expected noise %.4g from stacking %d batches with average noise %.4g\n",
					expectedNoise, int(numBatches), avgNoise )
	}

    // write out results, then free memory for the overall stack
	err=stack.WriteFile(stackP.OutName)
	if err!=nil { LogFatalf("Error writing file: %s\n", err) }
	stack=nil
}



// Stack a given batch of files, using the reference provided, or selecting a reference frame if nil.
// Returns the stack for the batch, and the reference frame. Updates stackP with sigma bounds if found.
func stackBatch(ids []int, fileNames []string, darkF, flatF *FITSImage, refFrame *FITSImage, 
	            preP *PreProcessParams, postP *PostProcessParams, stackP *StackParams, 
	            imageLevelParallelism int32) (stack, refFrameOut *FITSImage, avgNoise float32) {
	// Preprocess light frames (subtract dark, divide flat, remove bad pixels, detect stars and HFR)
	LogPrintf("\nPreprocessing %d frames with %s:\n", len(fileNames), preP)
	lights:=PreProcessLights(ids, fileNames, darkF, flatF, preP, imageLevelParallelism)
	debug.FreeOSMemory()					

	avgNoise=float32(0)
	for _,l:=range lights {
		avgNoise+=l.Stats.Noise
	}
	avgNoise/=float32(len(lights))
	LogPrintf("Average input frame noise is %.4g\n", avgNoise)

	// Select reference frame, unless one was provided from prior batches
	if (postP.Align!=0 || postP.NormHist!=0) && (refFrame==nil) {
		refFrameScore:=float32(0)
		refFrame, refFrameScore=SelectReferenceFrame(lights)
		if refFrame==nil { panic("Reference frame for alignment and normalization not found.") }
		LogPrintf("Using frame %d as reference. Score %.4g, %v.\n", refFrame.ID, refFrameScore, refFrame.Stats)
	}

	// Post-process all light frames (align, normalize)
	postP.OobMode=OobModeNaN
	LogPrintf("\nPostprocessing %d frames with %s:\n", len(lights), postP)
	PostProcessLights(refFrame, refFrame, lights, postP, imageLevelParallelism)
	debug.FreeOSMemory()					

	// Remove nils from lights
	o:=0
	for i:=0; i<len(lights); i+=1 {
		if lights[i]!=nil {
			lights[o]=lights[i]
			o+=1
		}
	}
	lights=lights[:o]

	// Prepare weights for stacking if applicable 
	weights:=[]float32(nil)
	if stackP.Weighted==1 { // exposure weighted stacking
		weights =make([]float32, len(lights))
		for i:=0; i<len(lights); i+=1 {
			if lights[i].Exposure==0 { LogFatalf("%d: Missing exposure information for exposure-weighted stacking", lights[i].ID) }
			weights[i]=lights[i].Exposure
		}
	} else if stackP.Weighted==2 { // noise weighted stacking
		minNoise, maxNoise:=float32(math.MaxFloat32), float32(-math.MaxFloat32)
		for i:=0; i<len(lights); i+=1 {
			n:=lights[i].Stats.Noise
			if n<minNoise { minNoise=n }
			if n>maxNoise { maxNoise=n }
		}		
		weights =make([]float32, len(lights))
		for i:=0; i<len(lights); i+=1 {
			lights[i].Stats.Noise=EstimateNoise(lights[i].Data, lights[i].Naxisn[0])
			weights[i]=1/(1+4*(lights[i].Stats.Noise-minNoise)/(maxNoise-minNoise))
		}
	}

	refFrameLoc:=float32(0)
	if refFrame!=nil && refFrame.Stats!=nil {
		refFrameLoc=refFrame.Stats.Location
	}

	// Stack the post-processed lights 
	if stackP.SigmaLow>=0 && stackP.SigmaHigh>=0 {
		// Use sigma bounds (given or from prior batch) for stacking
		LogPrintf("\nStacking %d frames with %s\n", len(lights), stackP)
		var err error
		stack, _, _, err=Stack(lights, weights, refFrameLoc, stackP)
		if err!=nil { LogFatal(err.Error()) }
	} else {
		// Find sigma bounds based on desired clipping percentages, and updates stackP with them
		LogPrintf("\nFinding sigmas for stacking %d frames with %s\n", len(lights), stackP)
		var err error
		stack, _, _, stackP.SigmaLow, stackP.SigmaHigh, err=FindSigmasAndStack(lights, weights, refFrameLoc, stackP)
		if err!=nil { LogFatal(err.Error()) }
	}

	// Free memory
	lights=nil
	debug.FreeOSMemory()

	return stack, refFrame, avgNoise
}
