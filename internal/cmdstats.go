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
	"runtime"
)


// Perform optional preprocessing and statistics
func CmdStats(fileNames []string, p *PreProcessParams) {
	// Set default parameters for this command
	if p.NormHist==HNMAuto { p.NormHist=HNMNone }
	if p.StarBpSig<0 { p.StarBpSig=5 } // default to noise elimination, we don't know if stats are called on single frame or resulting stack

    // Load dark and flat if flagged
	darkF, flatF, err:=LoadDarkAndFlat(p.Dark, p.Flat)
	if err!=nil { LogFatal(err) }

	// Preprocess light frames (subtract dark, divide flat, remove bad pixels, detect stars and HFR)
	LogPrintf("\nPreprocessing %d frames with %s:\n", len(fileNames), p)

	sem   :=make(chan bool, runtime.NumCPU())
	for id, fileName := range(fileNames) {
		sem <- true 
		go func(id int, fileName string) {
			defer func() { <-sem }()
			lightP, err:=PreProcessLight(id, fileName, darkF, flatF, p)
			if err!=nil {
				LogPrintf("%d: Error: %s\n", id, err.Error())
			} else {
				if p.PrePattern!="" {
					err=lightP.WriteFile(fmt.Sprintf(p.PrePattern, id))
					if err!=nil { LogFatalf("Error writing file: %s\n", err) }
				}
				if p.StarPattern!="" {
					starsFits:=ShowStars(lightP, 2.0)
					err=starsFits.WriteFile(fmt.Sprintf(p.StarPattern, id))
					if err!=nil { LogFatalf("Error writing file: %s\n", err) }
					starsFits.Data=nil
				}
				lightP.Data=nil
			}
		}(id, fileName)
	}
	for i:=0; i<cap(sem); i++ {  // wait for goroutines to finish
		sem <- true
	}
}

