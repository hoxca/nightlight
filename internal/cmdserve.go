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
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/contrib/static"
	"net/http"
	"fmt"
	"os"
)


var inputs            []string
var preProcessParams  *PreProcessParams
var postProcessParams *PostProcessParams
var stackParams       *StackParams
var colorParams       *ColorParams
var toneCurveParams   *ToneCurveParams
var output            string
var jpgOutput         string

// Serve static web content and API endpoints via HTTP
func CmdServe(port int, preP *PreProcessParams, postP *PostProcessParams, stackP *StackParams, 
	          colorP *ColorParams, toneP *ToneCurveParams, fileNames []string, out string, jpg string) {
	// set parameters
	inputs=fileNames
	preProcessParams=preP
	postProcessParams=postP
	stackParams=stackP
	colorParams=colorP
	toneCurveParams=toneP
	output=out
	jpgOutput=jpg

	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	//gin.SetMode(gin.ReleaseMode())

 	// Serve frontend static files
  	r.Use(static.Serve("/", static.LocalFile("./web/dist", true)))


  	// Serve backend APIs
  	api:=r.Group("/api/v1")
  	api.GET("/dir/*path",          getDir)

	api.GET("/preprocess/inputs",  getPreProcessInputs)
	api.GET("/preprocess/params",  getPreProcessParams)
	api.GET("/postprocess/params", getPostProcessParams)
	api.GET("/stack/params",       getStackParams)
	api.GET("/color/params",       getColorParams)
	api.GET("/tonecurve/params",   getToneCurveParams)

	api.POST("/preprocess/inputs",  postPreProcessInputs)
	api.POST("/preprocess/params",  postPreProcessParams)
	api.POST("/postprocess/params", postPostProcessParams)
	api.POST("/stack/params",       postStackParams)
	api.POST("/color/params",       postColorParams)
	api.POST("/tonecurve/params",   postToneCurveParams)

	api.POST("/stack/run",         postStackRun)

	// listen and serve on 0.0.0.0:port (for windows "localhost:port")	
	r.Run(fmt.Sprintf(":%d", port)) 
}

// Returns list of the directories and files in the given directory as JSON
func getDir(c *gin.Context) {
	dir, err:=os.Open(c.Param("path"))
	if err!=nil { 
		c.JSON(http.StatusNotFound, gin.H{"error":err.Error()}) 
		return
	}	

	fis, err:=dir.Readdir(0)
	dir.Close()
	if err!=nil { 
		c.JSON(http.StatusNotFound, gin.H{"error":err.Error()}) 
		return
	}

	files:=make([]string, len(fis))
	dirs :=make([]string, len(fis))
	numFiles, numDirs:=0,0
	for _,fi:=range fis {
		if (fi.Mode()&os.ModeDir)!=0 {
			dirs[numDirs]=fi.Name()
			numDirs++
		} else {
			files[numFiles]=fi.Name()
			numFiles++
		}
	}

	c.JSON(http.StatusOK, gin.H{"dirs" :dirs [:numDirs ], 
		                        "files":files[:numFiles] } )		
}	

func getPreProcessInputs(c *gin.Context) {
	c.JSON(http.StatusOK, inputs)
}

func postPreProcessInputs(c *gin.Context) {
	if err:=c.BindJSON(&inputs); err==nil {
		c.JSON(http.StatusOK, gin.H{"status":"ok"} )
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error":err.Error()})
	}
}


func getPreProcessParams(c *gin.Context) {
	c.JSON(http.StatusOK, *preProcessParams)
}

func postPreProcessParams(c *gin.Context) {
	if err:=c.BindJSON(preProcessParams); err==nil {
		c.JSON(http.StatusOK, gin.H{"status":"ok"} )
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error":err.Error()})
	}
}


func getPostProcessParams(c *gin.Context) {
	c.JSON(http.StatusOK, *postProcessParams)
}

func postPostProcessParams(c *gin.Context) {
	if err:=c.BindJSON(postProcessParams); err==nil {
		c.JSON(http.StatusOK, gin.H{"status":"ok"} )
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error":err.Error()})
	}
}


func getStackParams(c *gin.Context) {
	c.JSON(http.StatusOK, *stackParams)
}

func postStackParams(c *gin.Context) {
	if err:=c.BindJSON(stackParams); err==nil {
		c.JSON(http.StatusOK, gin.H{"status":"ok"} )
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error":err.Error()})
	}
}


func getColorParams(c *gin.Context) {
	c.JSON(http.StatusOK, *colorParams)
}

func postColorParams(c *gin.Context) {
	if err:=c.BindJSON(colorParams); err==nil {
		c.JSON(http.StatusOK, gin.H{"status":"ok"} )
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error":err.Error()})
	}
}


func getToneCurveParams(c *gin.Context) {
	c.JSON(http.StatusOK, *toneCurveParams)
}

func postToneCurveParams(c *gin.Context) {
	if err:=c.BindJSON(toneCurveParams); err==nil {
		c.JSON(http.StatusOK, gin.H{"status":"ok"} )
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error":err.Error()})
	}
}


func postStackRun(c *gin.Context) {
	w := c.Writer
	header := w.Header()
	header.Set("Transfer-Encoding", "chunked")
	header.Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)

	w.WriteString("Starting\n")
	CmdStack(inputs, preProcessParams, postProcessParams, stackParams)
	w.WriteString("Done\n")

	w.(http.Flusher).Flush()		
}

