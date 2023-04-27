/*******************************************************************
对外提供API或函数供上层调用，扫描代码库每个函数的分支点。
 ********************************************************************/
package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/ioutil"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type StructInfo struct {
	VarInfo  string
	TypeInfo string
}

type VarStruct struct {
	Name      string
	StructMsg []StructInfo
}

type CfgInFun struct {
	Line    int
	CodeMsg string
}

var varStructMap map[string][]VarStruct
var fileFuncMutex sync.Mutex
var fileFunc map[string]interface{}
var fileList []string

func ParseGenDelType(genDel *ast.GenDecl, packageName string) {
	//todo 是否需要const val、import package
	if genDel.Tok.String() == "type" {
		for _, varIn := range genDel.Specs {
			varSt := VarStruct{}
			varSt.Name = varIn.(*ast.TypeSpec).Name.Name

			if v, ok := varIn.(*ast.TypeSpec).Type.(*ast.StructType); ok {
				buf := new(bytes.Buffer)
				if err := format.Node(buf, token.NewFileSet(), v); err == nil {
					//fmt.Println("packageName ", packageName, " type ", varSt.Name, buf.String())
					varSt.StructMsg = append(varSt.StructMsg, StructInfo{
						VarInfo: buf.String(),
					})
				}
			}
		}
	}
}

func formatNode(node ast.Node) string {
	buf := new(bytes.Buffer)
	if err := format.Node(buf, token.NewFileSet(), node); err != nil {
		return ""
	}

	return strings.Split(buf.String(), "\n")[0]
}

func parserStmt(stmt ast.Stmt, funCfg map[string][]CfgInFun, funDelStr string, fset *token.FileSet) {
	switch v := stmt.(type) {
	case *ast.IfStmt:
		tmp := CfgInFun{
			Line:    fset.Position(v.Pos()).Line,
			CodeMsg: formatNode(v),
		}

		funCfg[funDelStr] = append(funCfg[funDelStr], tmp)
		//fmt.Println(id, " if Stmt: ", formatNode(stmt))

		for _, el := range v.Body.List {
			parserStmt(el, funCfg, funDelStr, fset)
		}

		// error
		if elseBlock, ok := v.Else.(*ast.BlockStmt); ok {
			tmp := CfgInFun{
				Line:    fset.Position(elseBlock.Pos()).Line,
				CodeMsg: "else ",
			}

			funCfg[funDelStr] = append(funCfg[funDelStr], tmp)
			//fmt.Println(id, " if Else Stmt: ", formatNode(elseBlock))

			for _, el := range elseBlock.List {
				parserStmt(el, funCfg, funDelStr, fset)
			}
		}
		if ifStmt, ok := v.Else.(*ast.IfStmt); ok {
			parserStmt(ifStmt, funCfg, funDelStr, fset)
		}

	case *ast.ForStmt:
		for _, el := range v.Body.List {
			parserStmt(el, funCfg, funDelStr, fset)
		}
		parserStmt(v.Post, funCfg, funDelStr, fset)

	case *ast.RangeStmt:
		if v.Body != nil {
			for _, e := range v.Body.List {
				parserStmt(e, funCfg, funDelStr, fset)
			}
		}

	case *ast.SelectStmt:
		for _, el := range v.Body.List {
			if caseEl, ok := el.(*ast.CommClause); ok {
				tmp := CfgInFun{
					Line:    int(v.Pos()),
					CodeMsg: formatNode(caseEl),
				}

				funCfg[funDelStr] = append(funCfg[funDelStr], tmp)

				//fmt.Println(id, "SelectStmt CommClause Stmt: ", formatNode(caseEl))
				for _, v := range caseEl.Body {
					parserStmt(v, funCfg, funDelStr, fset)
				}
			}
		}

	case *ast.SwitchStmt:
		for _, el := range v.Body.List {
			if caseEl, ok := el.(*ast.CaseClause); ok {
				tmp := CfgInFun{
					Line:    int(v.Pos()),
					CodeMsg: formatNode(caseEl),
				}

				funCfg[funDelStr] = append(funCfg[funDelStr], tmp)
				//fmt.Println(id, "SwitchStmt CaseClause Stmt: ", formatNode(caseEl))

				for _, v := range caseEl.Body {
					parserStmt(v, funCfg, funDelStr, fset)
				}
			}
		}

	case *ast.ExprStmt:
		if callFun, ok := v.X.(*ast.CallExpr); ok {
			for _, v := range callFun.Args {
				if vv, ok := v.(*ast.FuncLit); ok {
					for _, e := range vv.Body.List {
						parserStmt(e, funCfg, funDelStr, fset)
					}
				}
			}
		}
	}
}

func ParseFunc(fun *ast.FuncDecl, funCfg map[string][]CfgInFun, funDelStr string, fset *token.FileSet) {
	for _, stmt := range fun.Body.List {
		parserStmt(stmt, funCfg, funDelStr, fset)
	}
}

func getFileList(dir string) error {
	if dir == "." {
		str, err := os.Getwd()
		if err != nil {
			fmt.Println("get cwd err ", err)
			return err
		}
		dir = str
	}

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		fmt.Println("ReadDir err ", err)
		return err
	}

	for _, file := range files {
		if file.IsDir() {
			getFileList(dir + "/" + file.Name())
		} else {
			if strings.HasSuffix(file.Name(), ".go") && !strings.HasSuffix(file.Name(), "_test.go") {
				fileList = append(fileList, dir+"/"+file.Name())
			}
		}
	}

	return nil
}

func getLogId() uint64 {
	var x = strconv.Itoa((time.Now().Nanosecond() / 1000))
	res, _ := strconv.ParseInt(x, 10, 64)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	id := ((time.Now().Unix()*100000+res)&0xFFFFFFFF)*1000000000 + 100000000 + r.Int63n(899999999)
	return uint64(id)
}

/*
 {errno, logid}
*/
func Analysis() {
	asynAnalysis(0, fileList)
	for k, v := range fileFunc {
		fmt.Println("通过解析到的结果：", k, v)
	}
}

func asynAnalysis(logid uint64, fileList []string) {
	fmt.Println(len(fileList))
	fileList = append(fileList, "test/te.go")
	varStructMap = make(map[string][]VarStruct, 1024)
	fileFunc = make(map[string]interface{}, 1024)

	begin := time.Now().Unix()

	for _, file := range fileList {
		fmt.Println("file path : ", file)

		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			fmt.Println("parser file error ", f.Name.Name)
			return
		}

		packageName := f.Name.Name
		//fmt.Println("packageName: ", packageName)

		funCfgMap := make(map[string][]CfgInFun, 128)

		fileFuncMutex.Lock()
		fileFunc[file] = funCfgMap
		fileFuncMutex.Unlock()

		ast.Inspect(f, func(x ast.Node) bool {
			if fun, ok := x.(*ast.FuncDecl); ok {
				//函数声明的地方
				funcDecl := formatNode(x)
				//fmt.Println("fun line: ", fset.Position(x.Pos()).Line, " funcDecl ", funcDecl)
				funDecl := funcDecl[0:strings.LastIndex(funcDecl, " {")]
				//fmt.Println("funcName  ", funcDecl)

				funCfgMap[funDecl] = []CfgInFun{}
				ParseFunc(fun, funCfgMap, funDecl, fset)
			}

			if genDel, ok := x.(*ast.GenDecl); ok {
				ParseGenDelType(genDel, packageName)
			}

			return true
		})

		for k, v := range funCfgMap {
			fmt.Println("path ", k)
			fmt.Println("v ", len(v))
			for _, vv := range v {
				fmt.Println("line ", vv.Line, " code ", vv.CodeMsg)
			}
		}
	}

	end := time.Now().Unix()
	fmt.Println("Analysis used ", end-begin, " second len ", len(fileFunc))
}

func main12() {
	//  httpApiServ := http.NewServeMux()
	//  httpApiServ.HandleFunc("/rest/2.0/pvcp/analysis", Analysis)

	//  http.ListenAndServe("127.0.0.1:8000", httpApiServ)
	Analysis()
	
}
