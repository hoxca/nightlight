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
	"path/filepath"
)


// Turn filename wildcards into list of light frame files
func GlobFilenameWildcards(args []string) []string {
	fileNames:=[]string{}
	if args==nil { return fileNames }
	
	for _, pattern := range args {
		matches, err := filepath.Glob(pattern)
		if err!=nil { LogFatal(err) }
		fileNames=append(fileNames, matches...)
	}
	return fileNames
}

// Helper: convert bool to int
func Btoi(b bool) int {
	if b { return 1 }
	return 0
}
