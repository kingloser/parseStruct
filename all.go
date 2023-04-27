package main

import (
	"fmt"
	"path"
	"runtime"
)
func main1111(){
	fmt.Println("----?")

	fileName,funcName,line:=getLocation(9)
	fmt.Println(fileName)
	fmt.Println(funcName)
	fmt.Println(line)


}

func getLocation(skip int)(fileName ,funcName string ,line int){
	pc, file, line, ok := runtime.Caller(skip)
	if !ok {
		fmt.Println("get info failed")
		return
	}
	fmt.Println(pc,file)
	fileName = path.Base(file)
	funcName = runtime.FuncForPC(pc).Name()
	return
}