package main

import (
	"errors"
	"fmt"
	"go/ast"

	// "go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"log"
	"os"
	"reflect"
	// "strings"
)

//   A  -->   <---  B  --->   <--- c
type NodeStruct struct {
	Node interface{} //  node 指针

	Next    interface{} // 指向下一个影响的指针
	UpNode  interface{} // 上一个node
	KeyInfo string      // 记录该节点的所包含的核心信息
	Extra   string      // 备用

}

//  定义一个二维slice用来存储没有覆盖的行和列
var section [][]int

//  15 - 22  是一个switch  语句
const startLine = 33
const endLine = 34

var codePath []string
var ptrNode interface{}

//  设计 两个数据结构
//  1： map  来单纯存储 第一个 影响的参数，并标记搜索的深度
//  双向链表 ---》用来存储其拓扑关系

var NodeMap map[any]int // 将抓取到的 node 指针作为 key ，把扩散深度作为 value

//
func main() {
	// codePathTmp := "test/baidu/netdisk/pcs-go-pcsapi/models/service/file_delete.go"
	codePathTmp := "./test/te.go"
	section = append(section, []int{11, 11})
	astAnaly(codePathTmp)

}

var ff *token.FileSet

//  执行ast  分析
func astAnaly(codePath string) {
	fmt.Println("*****")
	// 解析源文件
	fset := token.NewFileSet()
	ff = fset
	f, err := parser.ParseFile(fset, codePath, nil, 0)
	if err != nil {
		panic(err)
	}

	ast.Inspect(f, func(node ast.Node) bool {
		err, start, end := getLine(node, fset)
		if err == nil {
			// 进行粗粒度的代码行未覆盖的检测，只有没有覆盖 才会进入解析函数
			FatherFlag, sec := sectionBig(section, start, end, true)
			FatherFlag = true
			if FatherFlag == true {
				er := findKeyNode(node, fset)
				if er != nil {
					fmt.Println(sec)
				}
			}
		}
		return true
	})

}

// 判断该定义是都为err类型，如果是返回true ，否则的化返回false
func isError_new(v ast.Expr) bool {
	// 这里是针对函数返回的err类型进行判断
	if expa, ok := v.(*ast.CallExpr); ok {
		if expb, ok := expa.Fun.(*ast.Ident); ok {
			if expc, ok := expb.Obj.Decl.(*ast.FuncDecl); ok {
				if expd, ok := expc.Type.Results.List[0].Type.(*ast.Ident); ok {
					if expd.Name == "error" {
						return true
					}
				}
			}
		}
	}

	if expa, ok := v.(*ast.CallExpr); ok {
		if expb, ok := expa.Fun.(*ast.SelectorExpr); ok {
			fmt.Println(expb)
			if expc, ok := expb.X.(*ast.Ident); ok {
				if expc.Name == "errors" {
					return true

				}
			}

		}

	}
	fmt.Println()
	return false
}

// 判断该定义是都为err类型，如果是返回true ，否则的化返回false
func isError(v ast.Expr, info *types.Info) bool {
	if expr, ok := v.(*ast.Ident); ok {
		fmt.Println("--------> reflect  ---> ", reflect.TypeOf(expr.Obj))
	}

	if intf, ok := info.TypeOf(v).Underlying().(*types.Interface); ok {
		return intf.NumMethods() == 1 && intf.Method(0).FullName() == "(error).Error"
	}
	return false
}

//  根据node返回该节点的起止行，如果有
func getLine(node ast.Node, fset *token.FileSet) (error, int, int) {
	if node != nil {
		posIsValid := node.Pos().IsValid()
		if posIsValid == true {
			//获取其实和结束行
			startLine := fset.Position(node.End()).Line
			endLine := fset.Position(node.Pos()).Line
			return nil, startLine, endLine
		}
		return errors.New("-1"), -1, -1
	}
	return errors.New("-1"), -1, -1
}

// 判断是不是一个日志打印，如果是日志的打印，则认为不需要处理
func hasLogFatal(stmt *ast.BlockStmt) bool {
	for i := len(stmt.List) - 1; i >= 0; i-- {
		if expr, ok := stmt.List[i].(*ast.ExprStmt); ok {
			if call, ok := expr.X.(*ast.CallExpr); ok {
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
					if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "log" {
						return true
					}
				}
			}
		}
	}
	return false
}

// 判断是不是一个日志打印，如果是日志的打印，则认为不需要处理
func isLog(stmt *ast.ExprStmt) bool {
	logInfo := make(map[string]bool)
	//  包含的日志打印类型
	logInfo["Notice"] = true
	logInfo["Error"] = true
	logInfo["Warning"] = true
	logInfo["Warn"] = true

	logInfo["Trace"] = true
	logInfo["log"] = true
	logInfo["Info"] = true
	if call, ok := stmt.X.(*ast.CallExpr); ok {
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if ident := sel.Sel; ident != nil {
				tmp := ident.Name
				if logInfo[tmp] == true {
					fmt.Println("====日志打印识别正确=====")
					return true
				}
			}
		}
	}

	// if call, ok := stmt.X.(*ast.CallExpr); ok {
	// 	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
	// 		if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "log" {
	// 			return true
	// 		}
	// 	}
	// }
	return false
}
func dealIf(node *ast.IfStmt) {
	var sh *ast.AssignStmt
	if _, ok := node.Cond.(*ast.BinaryExpr); ok {
		if _, ok := node.Cond.(*ast.BinaryExpr).X.(*ast.Ident); ok {
			if node.Cond.(*ast.BinaryExpr).X.(*ast.Ident).Obj != nil {
				switch condRight := node.Cond.(*ast.BinaryExpr).X.(*ast.Ident).Obj.Decl.(type) {
				case *ast.AssignStmt:
					sh = condRight
				}
			}
		}
	}
	// 获取到条件左边的if 语句的条件  为了防止panic，直接对所有路径下的类型进行断言，以避免整个程序panic
	if _, ok := node.Cond.(*ast.BinaryExpr); ok {
		if _, ok := node.Cond.(*ast.BinaryExpr).Y.(*ast.Ident); ok {
			if node.Cond.(*ast.BinaryExpr).Y.(*ast.Ident).Obj != nil {
				switch condRight := node.Cond.(*ast.BinaryExpr).Y.(*ast.Ident).Obj.Decl.(type) {
				case *ast.AssignStmt:
					if sh == nil {
						sh = condRight
					}

				}
			}
		}
	}
	// fmt.Println("此处 正常进行err类型 的判断")
	// if isError_new(sh.Rhs[0]) {
	// 	fmt.Println("----这是e一个err类型：  ")
	// 	// 然后再执行基于if语句的分析
	// }
	//  分析if 语句，是否为核心的操作
	oper := node.Body.List
	fmt.Println(len((oper)))
	//  对于if 语句中的节点进行逐个分析
	var flag = 0
	var notIgnore []int

	for index, v := range oper {
		switch m := v.(type) {
		case *ast.ExprStmt:
			if isLog(m) {
				flag++
			}
		case *ast.ReturnStmt:
			flag++
		default:
			notIgnore = append(notIgnore, index)
		}
	}
	if flag == len(oper) {
		fmt.Println("该if 语句只有日志打印和return 语句，低风险")

	}
	_, start, end := getLine(node, ff)
	fmt.Println(start, end)
	fmt.Println(notIgnore)
}
func findKeyNode(node ast.Node, fset *token.FileSet) *ast.AssignStmt {

	switch v := node.(type) {
	case *ast.IfStmt:
		return nil

	case *ast.ForStmt:
		//
		fmt.Println("--这是一个for循环")
	case *ast.RangeStmt:
		//
		fmt.Println("----range 语句")
	case *ast.SelectStmt:
		//
		fmt.Println("select  语句")
	case *ast.SwitchStmt:
		//
		fmt.Println("--这是一个swith的关键节点")

	case *ast.AssignStmt:
		//
		fmt.Println("ast.ExprStmt")
		//   先暂存一个c 来
		if ptrNode == nil && v.Lhs[0].(*ast.Ident).Obj.Name == "c" {
			ptrNode = v.Lhs[0].(*ast.Ident).Obj
		}
		//  还缺函数级别的判断
		switch dm := v.Rhs[0].(type) {
		case *ast.Ident:
			tmpNode := v.Rhs[0].(*ast.Ident).Obj
			if ptrNode != nil && ptrNode == tmpNode {
				fmt.Println("---->捕获了一个相等的指针")
			}
		case *ast.CallExpr:
			//  此时会发生函数的调用，目前简单一点，直接去解析整个入参
			if st, ok := v.Rhs[0].(*ast.CallExpr); ok {
				for index, data := range st.Args {
					tmpPtr := data.(*ast.Ident).Obj
					//  函数入参级别的捕获，需要将 入参 ----> 替换为函数内部的计算
					if tmpPtr != nil && tmpPtr == ptrNode {
						fmt.Println("函数级别捕获成功g", index)
					}
				}
			}
			if v.Rhs[0].(*ast.CallExpr).Fun.(*ast.Ident).Name == "test" {
				md := v.Rhs[0].(*ast.CallExpr).Fun.(*ast.Ident).Obj
				FuncNodeParse(md.Decl.(*ast.FuncDecl))
			}
		default:
			fmt.Println("----》", dm)
		}

	case *ast.CaseClause:
		fmt.Println("这是一个case语句")
	case *ast.FuncDecl:
		FuncNodeParse(node.(*ast.FuncDecl))
		_, start, end := getLine(v, fset)
		fmt.Println(v.Name.Name)
		fmt.Println("开始行：%d 结束行：%d", start, end)
		FatherFlag, sec := sectionBig(section, start, end, false)
		fmt.Println(FatherFlag, sec)
		fmt.Println("函数")
	case *ast.BlockStmt:
		// fmt.Println("---这是一个日志打印 ", hasLogFatal(v))

		// hasLogFatal(v)
	case *ast.ReturnStmt:
		fmt.Println("---这是一个return 语句")

	}
	return nil
}

//  新增 大区域的标记，如果bigflage为ture,则标记认为大范围，例如 10-20行的代码，在没有覆盖
// 的11-12行，也是
func sectionBig(sec [][]int, rLine int, leLine int, bigFlage bool) (bool, int) {
	for j, v := range sec {
		if len(v) != 2 {
			log.Printf("区间长度异常，不予处理")
			return false, -1
		}
		//  严格模式，主要过滤处严格在起止行列中的node信息
		if bigFlage == false {
			if rLine >= v[1] && leLine <= v[0] {
				return true, j
			}
		} else {
			if (v[0] >= leLine && v[0] <= rLine) || (rLine >= v[1] && leLine <= v[1]) {
				return true, j
			}
		}

	}
	return false, -1
}

//PathExists 判断一个文件或文件夹是否存在 ,避免解析不存在的文件出现panic
func PathExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return false
}

// node.Name.Obj.Decl.Type.Params.List   获取函数定义的入参
//   获取函数的入参
func FuncNodeParse(node *ast.FuncDecl) map[interface{}]bool {
	tmpObj := make(map[interface{}]bool)
	if name := node.Name; name != nil {
		if obj := name.Obj; obj != nil {
			if decl := obj.Decl; decl != nil {
				if ntype := decl.(*ast.FuncDecl).Type; ntype != nil {
					if params := ntype.Params; params != nil {
						if paramsList := params.List; paramsList != nil {
							for index, value := range paramsList {
								fmt.Println(index)
								fmt.Println(value)
								tmp := ExperParse(value)
								if tmp != nil {
									tmpObj[tmp] = true
									fmt.Println("---> 抓取到内部的 obj")
								}

							}
						}
					}
				}

			}
		}
	}
	return tmpObj
}

//  解析ast.field ，并返回其中的指针
func ExperParse(node *ast.Field) interface{} {
	if node != nil {
		if name := node.Names; name != nil {
			//  暂时取第一个，后续有样本集再去看为什么
			if obj := name[0].Obj; obj != nil {
				return obj
			}
		}
	}
	return nil
}
