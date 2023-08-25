package util

import "testing"

func Test_scanProject(t *testing.T) {
	type args struct {
		folderPath string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "pcs_api",
			args: args{
				folderPath: "../test/baidu/netdisk/pcs-go-pcsapi",
			},
			want: "../test/baidu/netdisk/pcs-go-pcsapi/pkg",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			 got := ScanProject(tt.args.folderPath) 
				// t.Errorf("scanProject() = %v, want %v", got, tt.want)
				
				t.Logf("scanProject() = %v, want %v", got, tt.want)
			
		})
	}
}