/*
Copyright 2026 The Kubernetes Authors.

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

package nodeapprovers

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVerifyKEP(t *testing.T) {
	testcases := []struct {
		name      string
		dir       string
		wantCount int
		wantRole  string
		wantUser  string
	}{
		{
			name:      "valid",
			dir:       "valid",
			wantCount: 0,
		},
		{
			name:      "missing reviewer",
			dir:       "missing-reviewer",
			wantCount: 1,
			wantRole:  reviewerRole,
			wantUser:  "someoneelse",
		},
		{
			name:      "missing approver",
			dir:       "missing-approver",
			wantCount: 1,
			wantRole:  approverRole,
			wantUser:  "someoneelse",
		},
		{
			name:      "no owners",
			dir:       "no-owners",
			wantCount: 1,
			wantRole:  reviewerRole,
			wantUser:  "tallclair",
		},
		{
			name:      "no markers",
			dir:       "no-markers",
			wantCount: 0,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			kepPath := filepath.Join("testdata", tc.dir, "kep.yaml")
			violations, err := VerifyKEP(kepPath)
			require.NoError(t, err)
			require.Len(t, violations, tc.wantCount, "violations: %v", violations)

			if tc.wantCount == 1 {
				require.Equal(t, tc.wantRole, violations[0].Role)
				require.Equal(t, tc.wantUser, violations[0].User)
				require.Equal(t, kepPath, violations[0].KEPPath)
			}
		})
	}
}

func TestVerifyAll(t *testing.T) {
	violations, err := VerifyAll("testdata")
	require.NoError(t, err)

	// missing-reviewer, missing-approver, and no-owners each contribute one
	// violation; valid and no-markers contribute none.
	require.Len(t, violations, 3, "violations: %v", violations)

	for _, v := range violations {
		require.NotEmpty(t, v.String())
	}
}
