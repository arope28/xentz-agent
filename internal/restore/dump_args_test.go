package restore

import "testing"

func TestParseDumpArgs(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantSnapshot string
		wantPath     string
		wantOutput   string
		wantErr      bool
	}{
		{
			name:         "positional only writes to stdout",
			args:         []string{"latest", "/data/file.txt"},
			wantSnapshot: "latest",
			wantPath:     "/data/file.txt",
		},
		{
			name:         "output flag before positionals",
			args:         []string{"--output", "/tmp/out.txt", "latest", "/data/file.txt"},
			wantSnapshot: "latest",
			wantPath:     "/data/file.txt",
			wantOutput:   "/tmp/out.txt",
		},
		{
			name:         "output flag after positionals (documented drill order)",
			args:         []string{"latest", "/data/file.txt", "--output", "/tmp/out.txt"},
			wantSnapshot: "latest",
			wantPath:     "/data/file.txt",
			wantOutput:   "/tmp/out.txt",
		},
		{
			name:         "output flag with equals sign",
			args:         []string{"latest", "/data/file.txt", "--output=/tmp/out.txt"},
			wantSnapshot: "latest",
			wantPath:     "/data/file.txt",
			wantOutput:   "/tmp/out.txt",
		},
		{
			name:    "missing path",
			args:    []string{"latest"},
			wantErr: true,
		},
		{
			name:    "no args",
			args:    nil,
			wantErr: true,
		},
		{
			name:    "output flag missing value",
			args:    []string{"latest", "/data/file.txt", "--output"},
			wantErr: true,
		},
		{
			name:    "too many positionals",
			args:    []string{"latest", "/data/file.txt", "/extra"},
			wantErr: true,
		},
		{
			name:    "unknown flag",
			args:    []string{"latest", "/data/file.txt", "--nope"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshotID, pathInSnapshot, outPath, err := parseDumpArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseDumpArgs(%v) expected error, got none", tt.args)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDumpArgs(%v): %v", tt.args, err)
			}
			if snapshotID != tt.wantSnapshot || pathInSnapshot != tt.wantPath || outPath != tt.wantOutput {
				t.Fatalf("parseDumpArgs(%v) = (%q, %q, %q), want (%q, %q, %q)",
					tt.args, snapshotID, pathInSnapshot, outPath,
					tt.wantSnapshot, tt.wantPath, tt.wantOutput)
			}
		})
	}
}
