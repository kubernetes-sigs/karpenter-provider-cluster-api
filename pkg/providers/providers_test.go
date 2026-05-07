/*
Copyright 2024 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package providers

import (
	"testing"
)

func TestParseMachineAnnotation(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantNS    string
		wantName  string
		wantError bool
	}{
		{
			name:     "valid annotation",
			input:    "default/my-machine",
			wantNS:   "default",
			wantName: "my-machine",
		},
		{
			name:     "valid annotation with dashes",
			input:    "kube-system/machine-abc-123",
			wantNS:   "kube-system",
			wantName: "machine-abc-123",
		},
		{
			name:      "empty string",
			input:     "",
			wantError: true,
		},
		{
			name:      "no slash",
			input:     "my-machine",
			wantError: true,
		},
		{
			name:      "too many slashes",
			input:     "a/b/c",
			wantError: true,
		},
		{
			name:      "empty namespace",
			input:     "/my-machine",
			wantError: true,
		},
		{
			name:      "empty name",
			input:     "default/",
			wantError: true,
		},
		{
			name:      "only slash",
			input:     "/",
			wantError: true,
		},
		{
			name:      "whitespace namespace",
			input:     "  /my-machine",
			wantError: true,
		},
		{
			name:      "whitespace name",
			input:     "default/  ",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns, name, err := ParseMachineAnnotation(tt.input)
			if tt.wantError {
				if err == nil {
					t.Errorf("expected error for input %q, got ns=%q name=%q", tt.input, ns, name)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error for input %q: %v", tt.input, err)
				return
			}
			if ns != tt.wantNS {
				t.Errorf("namespace: got %q, want %q", ns, tt.wantNS)
			}
			if name != tt.wantName {
				t.Errorf("name: got %q, want %q", name, tt.wantName)
			}
		})
	}
}
