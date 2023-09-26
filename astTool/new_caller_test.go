package asttool

import (
	"parseStruct/util"
	"testing"
)

func TestSignleCaller(t *testing.T) {
	codePathTmp := "../test/baidu/netdisk/pcs-go-pcsapi/action/file/copy.go"
	file := util.ScanProject(codePathTmp)
	// dm := FullScan(file)

	tests := []struct {
		name    string
		args    []string
		want    map[string]MoudleFull
		wantErr bool
	}{
		{
			name: "1",
			args: file,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SignleCallerq(tt.args)
			// log.Fatalf(got)
		})
	}
}
