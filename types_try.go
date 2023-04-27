package main

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"log"
)

type Program struct {
	fs   map[string]string
	ast  map[string]*ast.File
	pkgs map[string]*types.Package
	fset *token.FileSet
}

func NewProgram(fs map[string]string) *Program {
	return &Program{
		fs:   fs,
		ast:  make(map[string]*ast.File),
		pkgs: make(map[string]*types.Package),
		fset: token.NewFileSet(),
	}
}
func (p *Program) LoadPackage(path string) (pkg *types.Package, f *ast.File, err error) {
	if pkg, ok := p.pkgs[path]; ok {
		return pkg, p.ast[path], nil
	}
	fmt.Println("---->>>", p.fs[path])
	f, err = parser.ParseFile(p.fset, p.fs[path], nil, parser.AllErrors)
	if err != nil {
		return nil, nil, err
	}
	log.Println("==ast解析完毕")
	//  首先使用默认的
	conf := types.Config{Importer: importer.Default()}
	pkg, err = conf.Check(path, p.fset, []*ast.File{f}, nil)
	fmt.Println("这里是check的err ", err)
	// var st string
	// st = "icode.baidu.com/baidu/netdisk/pcs-go-pcsapi/conf"
	// mn := importer.Default()
	// importer.Lookup(st)
	// if err != nil {
	// 	conf = types.Config{Importer: importer.Lookup(st.string())} // 用 Program 作为包导入器
	// 	pkg, err = conf.Check(path, p.fset, []*ast.File{f}, nil)
	// }

	if err != nil {
		return nil, nil, err
	}

	p.ast[path] = f
	p.pkgs[path] = pkg
	return pkg, f, nil
}

func (p *Program) Import(path string) (*types.Package, error) {
	if pkg, ok := p.pkgs[path]; ok {
		return pkg, nil
	}
	pkg, _, err := p.LoadPackage(path)
	return pkg, err
}

func main1() {
	prog := NewProgram(map[string]string{

		"icode.baidu.com/baidu/netdisk/pcs-go-pcsapi/conf": "/Users/wangkaijie/Desktop/ast_try/parseStruct/test/baidu/netdisk/pcs-go-pcsapi/conf",
		"service": "test/delete.go",
	})

	_, _, err := prog.LoadPackage("icode.baidu.com/baidu/netdisk/pcs-go-pcsapi/conf")
	fmt.Println("----")
	if err != nil {
		log.Println(err)
	}
	pkg, f, err := prog.LoadPackage("service")
	fmt.Println(pkg, f)
	if err != nil {
		fmt.Println("-=----> 这里是一一个错误  ")
		log.Println(err)
		return
	}
	log.Println("---没有错误执行完毕了------》")
}
