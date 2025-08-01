package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	cloudquery_api "github.com/cloudquery/cloudquery-api-go"
	"github.com/cloudquery/cloudquery-api-go/auth"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSync(t *testing.T) {
	configs := []struct {
		name    string
		config  string
		shard   string
		err     []string
		summary []syncSummary
	}{
		{
			name:   "sync_success_sourcev1_destv0",
			config: "sync-success-sourcev1-destv0.yml",
		},
		{
			name:   "multiple_sources",
			config: "multiple-sources.yml",
			summary: []syncSummary{
				{
					CLIVersion:        "development",
					DestinationErrors: 0,
					DestinationName:   "test",
					DestinationPath:   "cloudquery/test",
					Resources:         13,
					SourceName:        "test",
					SourcePath:        "cloudquery/test",
					SourceTables:      []string{"test_paid_table", "test_some_table", "test_sub_table", "test_testdata_table"},
					ResourcesPerTable: map[string]uint64{
						"test_some_table":     1,
						"test_sub_table":      10,
						"test_testdata_table": 1,
						"test_paid_table":     1,
					},
					ErrorsPerTable: map[string]uint64{
						"test_some_table":     0,
						"test_sub_table":      0,
						"test_testdata_table": 0,
						"test_paid_table":     0,
					},
				},
				{
					CLIVersion:        "development",
					DestinationErrors: 0,
					DestinationName:   "test",
					DestinationPath:   "cloudquery/test",
					Resources:         13,
					SourceName:        "test2",
					SourcePath:        "cloudquery/test",
					SourceTables:      []string{"test_paid_table", "test_some_table", "test_sub_table", "test_testdata_table"},
					ResourcesPerTable: map[string]uint64{
						"test_some_table":     1,
						"test_sub_table":      10,
						"test_testdata_table": 1,
						"test_paid_table":     1,
					},
					ErrorsPerTable: map[string]uint64{
						"test_some_table":     0,
						"test_sub_table":      0,
						"test_testdata_table": 0,
						"test_paid_table":     0,
					},
				},
			},
		},
		{
			name:   "multiple_destinations",
			config: "multiple-destinations.yml",
		},
		{
			name:   "multiple_destinations_multiple_batching_writers",
			config: "multiple-destinations-multiple-batching-writers.yml",
		},
		{
			name:   "multiple_sources_destinations",
			config: "multiple-sources-destinations.yml",
			summary: []syncSummary{
				{
					CLIVersion:      "development",
					DestinationName: "test-1",
					DestinationPath: "cloudquery/test",
					Resources:       13,
					SourceName:      "test-1",
					SourcePath:      "cloudquery/test",
					SourceTables:    []string{"test_paid_table", "test_some_table", "test_sub_table", "test_testdata_table"},
					ResourcesPerTable: map[string]uint64{
						"test_some_table":     1,
						"test_sub_table":      10,
						"test_testdata_table": 1,
						"test_paid_table":     1,
					},
					ErrorsPerTable: map[string]uint64{
						"test_some_table":     0,
						"test_sub_table":      0,
						"test_testdata_table": 0,
						"test_paid_table":     0,
					},
				},
				{
					CLIVersion:      "development",
					DestinationName: "test-2",
					DestinationPath: "cloudquery/test",
					Resources:       13,
					SourceName:      "test-2",
					SourcePath:      "cloudquery/test",
					SourceTables:    []string{"test_paid_table", "test_some_table", "test_sub_table", "test_testdata_table"},
					ResourcesPerTable: map[string]uint64{
						"test_some_table":     1,
						"test_sub_table":      10,
						"test_testdata_table": 1,
						"test_paid_table":     1,
					},
					ErrorsPerTable: map[string]uint64{
						"test_some_table":     0,
						"test_sub_table":      0,
						"test_testdata_table": 0,
						"test_paid_table":     0,
					},
				},
			},
		},
		{
			name:   "different_backend_from_destination",
			config: "different-backend-from-destination.yml",
			summary: []syncSummary{
				{
					CLIVersion:      "development",
					DestinationName: "test1",
					DestinationPath: "cloudquery/test",
					Resources:       13,
					SourceName:      "test",
					SourcePath:      "cloudquery/test",
					SourceTables:    []string{"test_paid_table", "test_some_table", "test_sub_table", "test_testdata_table"},
					ResourcesPerTable: map[string]uint64{
						"test_some_table":     1,
						"test_sub_table":      10,
						"test_testdata_table": 1,
						"test_paid_table":     1,
					},
					ErrorsPerTable: map[string]uint64{
						"test_some_table":     0,
						"test_sub_table":      0,
						"test_testdata_table": 0,
						"test_paid_table":     0,
					},
				},
			},
		},
		{
			name:   "with_sync_group_id",
			config: "with-sync-group-id.yml",
			summary: []syncSummary{
				{
					CLIVersion:      "development",
					DestinationName: "test1",
					DestinationPath: "cloudquery/test",
					Resources:       13,
					SourceName:      "test",
					SourcePath:      "cloudquery/test",
					SourceTables:    []string{"test_paid_table", "test_some_table", "test_sub_table", "test_testdata_table"},
					SyncGroupID:     lo.ToPtr("sync_group_id_test"),
					ResourcesPerTable: map[string]uint64{
						"test_some_table":     1,
						"test_sub_table":      10,
						"test_testdata_table": 1,
						"test_paid_table":     1,
					},
					ErrorsPerTable: map[string]uint64{
						"test_some_table":     0,
						"test_sub_table":      0,
						"test_testdata_table": 0,
						"test_paid_table":     0,
					},
				},
			},
		},
		{
			name:   "with_sync_group_id_and_shard",
			config: "with-sync-group-id.yml",
			shard:  "1/2",
			summary: []syncSummary{
				{
					CLIVersion:      "development",
					DestinationName: "test1",
					DestinationPath: "cloudquery/test",
					// Less resources due to sharding
					Resources:    11,
					SourceName:   "test",
					SourcePath:   "cloudquery/test",
					SourceTables: []string{"test_paid_table", "test_some_table", "test_sub_table", "test_testdata_table"},
					SyncGroupID:  lo.ToPtr("sync_group_id_test"),
					ShardNum:     lo.ToPtr(1),
					ShardTotal:   lo.ToPtr(2),
					ResourcesPerTable: map[string]uint64{
						"test_some_table":     1,
						"test_sub_table":      10,
						"test_testdata_table": 0,
						"test_paid_table":     0,
					},
					ErrorsPerTable: map[string]uint64{
						"test_some_table":     0,
						"test_sub_table":      0,
						"test_testdata_table": 0,
						"test_paid_table":     0,
					},
				},
			},
		},
		{
			name:   "should fail with missing path error when path is missing",
			config: "sync-missing-path-error.yml",
			err:    []string{"Error: failed to validate destination test: path is required"},
		},
		{
			name:   "source exits immediately",
			config: "source-exits.yml",
			err:    []string{"rpc error: code = Unavailable desc = error reading from server"}, // rpc disconnection
		},
		{
			name:   "destination exits immediately",
			config: "destination-exits.yml",
			err:    []string{"write client returned error"},
		},
		{
			name:   "transformer exits immediately",
			config: "transformer-exits.yml",
			err: []string{
				"rpc error: code = Unavailable desc = error reading from server", // rpc disconnection
				"failed to sync v3 source test: EOF",
			},
		},
		{
			name:   "transformer errors immediately",
			config: "transformer-errors.yml",
			err: []string{
				"failed to sync v3 source test: rpc error: code = Internal desc = failing at the transformer stage according to spec requirements", // rpc disconnection
				"failed to sync v3 source test: EOF",
			},
		},
		{
			name:   "transformer succeeds",
			config: "transformer-succeeds.yml",
		},
		{
			name:   "source errors immediately",
			config: "source-errors.yml",
			summary: []syncSummary{
				{
					CLIVersion:      "development",
					Resources:       0,
					SourceErrors:    1,
					DestinationName: "test",
					DestinationPath: "cloudquery/test",
					SourceName:      "test",
					SourcePath:      "cloudquery/test",
					SourceTables:    []string{"test_some_table"},
					ResourcesPerTable: map[string]uint64{
						"test_some_table": 0,
					},
					ErrorsPerTable: map[string]uint64{
						"test_some_table": 0,
					},
				},
			},
		},
		{
			name:   "destination errors immediately",
			config: "destination-errors.yml",
			// TODO: https://github.com/cloudquery/cloudquery-issues/issues/2907
			// this is a mitigation for flakiness that we want to fix later, so that we can have
			// E2E tests right away.
			err: []string{
				"failed to sync v3 source test: write client returned error (insert)",
				"failed to sync v3 source test: failed to send insert: EOF",
			},
		},
	}
	_, filename, _, _ := runtime.Caller(0)
	currentDir := path.Dir(filename)

	for _, tc := range configs {
		t.Run(tc.name, func(t *testing.T) {
			testConfig := path.Join(currentDir, "testdata", tc.config)
			cmd := NewCmdRoot()

			baseArgs := testCommandArgs(t)
			argList := append([]string{"sync", testConfig}, baseArgs...)
			summaryPath := ""
			if len(tc.summary) > 0 {
				tmp := t.TempDir()
				summaryPath = path.Join(tmp, "/test/cloudquery-summary.jsonl")
				argList = append(argList, "--summary-location", summaryPath)
			}

			if tc.shard != "" {
				argList = append(argList, "--shard", tc.shard)
			}

			cmd.SetArgs(argList)
			err := cmd.Execute()
			if len(tc.err) > 0 {
				if !anyErrorMatched(err, tc.err) {
					t.Fatalf("expected error matching any of %v, got %v", tc.err, err)
				}
			} else {
				assert.NoError(t, err)
			}

			if len(tc.summary) > 0 {
				summaries := readSummaries(t, summaryPath)
				// Ignore random fields or fields that are updated over time
				diff := cmp.Diff(tc.summary, summaries, cmpopts.IgnoreFields(syncSummary{}, "SyncID", "DestinationVersion", "SourceVersion", "SyncTime"))
				for _, s := range summaries {
					assert.NotEmpty(t, s.SyncID)
					assert.NotEmpty(t, s.SyncTime)
					assert.NotEmpty(t, s.DestinationVersion)
					assert.NotEmpty(t, s.SourceVersion)
				}
				require.Empty(t, diff, "unexpected summaries: %v", diff)
			}

			// check that log was written and contains some lines from the plugin
			b, logFileError := os.ReadFile(baseArgs[3])
			logContent := string(b)
			require.NoError(t, logFileError, "failed to read cloudquery.log")
			require.NotEmpty(t, logContent, "cloudquery.log empty; expected some logs")
		})

		t.Run(tc.name+"_no_migrate", func(t *testing.T) {
			testConfig := path.Join(currentDir, "testdata", tc.config)

			cmd := NewCmdRoot()
			cmd.SetArgs(append([]string{"sync", testConfig, "--no-migrate"}, testCommandArgs(t)...))
			err := cmd.Execute()
			if len(tc.err) > 0 {
				if !anyErrorMatched(err, tc.err) {
					t.Fatalf("expected error matching any of %v, got %v", tc.err, err)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func anyErrorMatched(err error, expectedErrors []string) bool {
	if err == nil {
		return false
	}
	for _, e := range expectedErrors {
		if strings.Contains(err.Error(), e) {
			return true
		}
	}
	return false
}

type syncSummaryTable struct {
	CLIVersion          string   `json:"cli_version"`
	DestinationErrors   uint64   `json:"destination_errors"`
	DestinationName     string   `json:"destination_name"`
	DestinationPath     string   `json:"destination_path"`
	DestinationVersion  string   `json:"destination_version"`
	DestinationWarnings uint64   `json:"destination_warnings"`
	Resources           uint64   `json:"resources"`
	SourceErrors        uint64   `json:"source_errors"`
	SourcePath          string   `json:"source_path"`
	SourceVersion       string   `json:"source_version"`
	SourceWarnings      uint64   `json:"source_warnings"`
	SourceTables        []string `json:"source_tables"`
	SyncID              string   `json:"sync_id"`
	ShardNum            *int     `json:"shard_num,omitempty"`
	ShardTotal          *int     `json:"shard_total,omitempty"`
	// Internal columns are prefixed with _cq_ in the destination schema (hence in the file destination JSON)
	SyncGroupID *string `json:"_cq_sync_group_id,omitempty"`
	SyncTime    string  `json:"_cq_sync_time"`
	SourceName  string  `json:"_cq_source_name"`
}

func TestSyncWithSummaryTable(t *testing.T) {
	configs := []struct {
		name         string
		config       string
		shard        string
		err          string
		summaryTable []syncSummaryTable
	}{
		{
			name:   "with-destination-summary",
			config: "with-destination-summary.yml",
			summaryTable: []syncSummaryTable{
				{
					CLIVersion:         "development",
					DestinationErrors:  0,
					DestinationName:    "test",
					DestinationPath:    "cloudquery/file",
					DestinationVersion: "v5.2.5",
					Resources:          13,
					SourceName:         "test",
					SourcePath:         "cloudquery/test",
					SourceVersion:      "v4.5.1",
					SourceTables:       []string{"test_paid_table", "test_some_table", "test_sub_table", "test_testdata_table"},
				},
			},
		},
		{
			name:   "with-destination-summary-with-sync-group-id-and-shard",
			config: "with-destination-summary-with-sync-group-id-and-shard.yml",
			shard:  "1/2",
			summaryTable: []syncSummaryTable{
				{
					CLIVersion:         "development",
					DestinationErrors:  0,
					DestinationName:    "test",
					DestinationPath:    "cloudquery/file",
					DestinationVersion: "v5.2.5",
					// Less resources due to sharding
					Resources:     11,
					SourceName:    "test_1_2",
					SourcePath:    "cloudquery/test",
					SourceVersion: "v4.5.1",
					SourceTables:  []string{"test_paid_table", "test_some_table", "test_sub_table", "test_testdata_table"},
					SyncGroupID:   lo.ToPtr("sync_group_id_test"),
					ShardNum:      lo.ToPtr(1),
					ShardTotal:    lo.ToPtr(2),
				},
			},
		},
	}
	_, filename, _, _ := runtime.Caller(0)
	currentDir := path.Dir(filename)

	for _, tc := range configs {
		t.Run(tc.name, func(t *testing.T) {
			testConfig := path.Join(currentDir, "testdata", tc.config)
			cmd := NewCmdRoot()
			baseArgs := testCommandArgs(t)
			argList := append([]string{"sync", testConfig}, baseArgs...)

			summaryTablePath := ""
			if len(tc.summaryTable) > 0 {
				datadir := t.TempDir()
				summaryTablePath = path.Join(datadir, "/data/cloudquery_sync_summaries")
				// this is the only way to inject the dynamic output path
				os.Setenv("CQ_FILE_DESTINATION", path.Join(datadir, "/data/{{TABLE}}/{{UUID}}.{{FORMAT}}"))
			}
			if tc.shard != "" {
				argList = append(argList, "--shard", tc.shard)
			}
			cmd.SetArgs(argList)
			err := cmd.Execute()
			if tc.err != "" {
				assert.Contains(t, err.Error(), tc.err)
			} else {
				assert.NoError(t, err)
			}
			summaries := []syncSummaryTable{}
			// find all json files in the data directory
			files, err := os.ReadDir(summaryTablePath)
			if err != nil {
				t.Fatalf("failed to read directory %v: %v", summaryTablePath, err)
			}
			for _, file := range files {
				if file.IsDir() {
					continue
				}
				b, err := os.ReadFile(path.Join(summaryTablePath, file.Name()))
				if err != nil {
					t.Fatalf("failed to read file %v: %v", file.Name(), err)
				}
				var v syncSummaryTable
				assert.NoError(t, json.Unmarshal(b, &v))
				summaries = append(summaries, v)
			}

			// Ignore random fields or fields that are updated over time
			diff := cmp.Diff(tc.summaryTable, summaries, cmpopts.IgnoreFields(syncSummaryTable{}, "SyncID", "SyncTime", "DestinationVersion", "SourceVersion"))
			for _, s := range summaries {
				assert.NotEmpty(t, s.SyncID)
				assert.NotEmpty(t, s.SyncTime)
				assert.NotEmpty(t, s.DestinationVersion)
				assert.NotEmpty(t, s.SourceVersion)
			}

			require.Empty(t, diff, "unexpected summaries: %v", diff)

			// have to ignore SyncID because it's random and plugin versions since we update those frequently using an automated process
			// also ignore SyncTime because it's a timestamp
			for _, s := range summaries {
				assert.NotEmpty(t, s.SyncID)
			}
		})
	}
}

func TestSyncCqDir(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	currentDir := path.Dir(filename)
	testConfig := path.Join(currentDir, "testdata", "sync-success-sourcev1-destv0.yml")

	cmd := NewCmdRoot()
	baseArgs := testCommandArgs(t)
	cmd.SetArgs(append([]string{"sync", testConfig}, baseArgs...))
	err := cmd.Execute()
	require.NoError(t, err)

	// check that destination plugin was downloaded to the cache using --cq-dir
	p := path.Join(baseArgs[1], "plugins")
	files, err := os.ReadDir(p)
	if err != nil {
		t.Fatalf("failed to read cache directory %v: %v", p, err)
	}
	require.NotEmpty(t, files, "destination plugin not downloaded to cache")
}

func TestFindMaxCommonVersion(t *testing.T) {
	cases := []struct {
		name       string
		givePlugin []int
		giveCLI    []int
		want       int
	}{
		{name: "support_less", givePlugin: []int{1, 2, 3}, giveCLI: []int{1, 2}, want: 2},
		{name: "support_same", givePlugin: []int{1, 2, 3}, giveCLI: []int{1, 2, 3}, want: 3},
		{name: "support_more", givePlugin: []int{1, 2, 3}, giveCLI: []int{2, 3, 4}, want: 3},
		{name: "support_only_lower", givePlugin: []int{3, 4, 5}, giveCLI: []int{6, 7}, want: -1},
		{name: "support_only_higher", givePlugin: []int{3, 4, 5}, giveCLI: []int{1, 2}, want: -2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := findMaxCommonVersion(tc.givePlugin, tc.giveCLI)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestSync_IsolatedPluginEnvironmentsInCloud(t *testing.T) {
	configs := []struct {
		name   string
		config string
		err    string
	}{
		{
			name:   "source-with-env",
			config: "source-with-env.yml",
		},
	}
	_, filename, _, _ := runtime.Caller(0)
	currentDir := path.Dir(filename)

	t.Setenv("CQ_CLOUD", "1")
	t.Setenv("_CQ_TEAM_NAME", "cloudquery")
	t.Setenv("_CQ_SYNC_NAME", "test_sync")
	t.Setenv("_CQ_SYNC_RUN_ID", uuid.Must(uuid.NewUUID()).String())
	t.Setenv("__SOURCE_TEST__TEST_KEY", "test_value")
	t.Setenv("NOT_TEST_ENV", "should_not_be_visible_to_plugin")

	for _, tc := range configs {
		t.Run(tc.name, func(t *testing.T) {
			testConfig := path.Join(currentDir, "testdata", tc.config)
			cmd := NewCmdRoot()
			cmd.SetArgs(append([]string{"sync", testConfig}, testCommandArgs(t)...))
			err := cmd.Execute()
			if tc.err != "" {
				assert.Contains(t, err.Error(), tc.err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSync_RemoteProgressReporting(t *testing.T) {
	// save the original auth token so we can rewrite requests to the Hub
	authClient := auth.NewTokenClient()
	originalToken, err := authClient.GetToken()
	if err != nil {
		t.Fatalf("failed to get auth token: %v", err)
	}

	var (
		cqTeamName  = "cloudquery"
		cqSyncName  = "test_sync"
		cqSyncRunID = uuid.Must(uuid.NewUUID()).String()
	)

	progressReported := atomic.Bool{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		urlPathToMock := fmt.Sprintf("/teams/%s/syncs/%s/runs/%s/progress", cqTeamName, cqSyncName, cqSyncRunID)
		if r.URL.Path == urlPathToMock {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("failed to read body: %v", err)
			}
			var v cloudquery_api.CreateSyncRunProgressJSONRequestBody
			require.NoError(t, json.Unmarshal(body, &v))

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNoContent)
			_, _ = w.Write([]byte(`{}`))
			progressReported.Store(true)
			return
		}
		requestToCloud, _ := http.NewRequest(r.Method, "https://api.cloudquery.io"+r.URL.Path, r.Body)
		requestToCloud.Header = r.Header
		requestToCloud.Header.Set("Authorization", fmt.Sprintf("Bearer %s", originalToken.Value))
		resp, err := http.DefaultClient.Do(requestToCloud)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintf(w, "failed to read body: %v", err)
			return
		}
		switch resp.StatusCode {
		case http.StatusOK:
			w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
			w.Header().Set("Content-Encoding", resp.Header.Get("Content-Encoding"))
			w.WriteHeader(resp.StatusCode)
			defer resp.Body.Close()
			_, _ = io.Copy(w, resp.Body)
		default:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintf(w, "unexpected status code: %d for url %s", resp.StatusCode, r.URL.String())
		}
	}))
	defer ts.Close()

	_, filename, _, _ := runtime.Caller(0)
	currentDir := path.Dir(filename)

	t.Setenv("CQ_CLOUD", "1")
	t.Setenv("_CQ_TEAM_NAME", cqTeamName)
	t.Setenv("_CQ_SYNC_NAME", cqSyncName)
	t.Setenv("_CQ_SYNC_RUN_ID", cqSyncRunID)
	// The cqsr_ prefix is important so the CLI reports progress to the API
	t.Setenv("CLOUDQUERY_API_KEY", "cqsr_test-api-key")
	t.Setenv("CLOUDQUERY_API_URL", ts.URL)

	testConfig := path.Join(currentDir, "testdata", "with-remote-progress.yml")
	cmd := NewCmdRoot()
	cmd.SetArgs(append([]string{"sync", testConfig}, testCommandArgs(t)...))
	require.NoError(t, cmd.Execute())
	require.True(t, progressReported.Load(), "expected progress to be reported")
}

func TestSync_FilterPluginEnv(t *testing.T) {
	cases := []struct {
		Name        string
		TotalEnv    []string
		FilteredEnv []string
	}{
		{
			Name: "double_api_keys",
			TotalEnv: []string{
				"CLOUDQUERY_API_KEY=outer-key",
				"__SOURCE_TEST__TEST_KEY=test_value",
				"__SOURCE_TEST__CLOUDQUERY_API_KEY=inner-key",
				"NOT_TEST_ENV=should_not_be_visible_to_plugin",
			},
			FilteredEnv: []string{
				"TEST_KEY=test_value",
				"CLOUDQUERY_API_KEY=inner-key",
			},
		},
		{
			Name: "single_api_key",
			TotalEnv: []string{
				"CLOUDQUERY_API_KEY=outer-key",
				"__SOURCE_TEST__TEST_KEY=test_value",
				"NOT_TEST_ENV=should_not_be_visible_to_plugin",
			},
			FilteredEnv: []string{
				"TEST_KEY=test_value",
				"CLOUDQUERY_API_KEY=outer-key",
			},
		},
		{
			Name: "global_aws_values_get_overwritten",
			TotalEnv: []string{
				"AWS_ROLE_ARN=arn:aws:iam::123456789012:role/role-name",
				"__SOURCE_TEST__AWS_ROLE_ARN=arn:aws:iam::123456789012:role/role-name-2",
			},
			FilteredEnv: []string{
				"AWS_ROLE_ARN=arn:aws:iam::123456789012:role/role-name-2",
			},
		},
		{
			Name: "aws_values_use_global_defaults",
			TotalEnv: []string{
				"AWS_REGION=us-east-1",
				"AWS_ROLE_ARN=arn:aws:iam::123456789012:role/role-name",
				"__SOURCE_TEST__AWS_ROLE_ARN=arn:aws:iam::123456789012:role/role-name-2",
			},
			FilteredEnv: []string{
				"AWS_REGION=us-east-1",
				"AWS_ROLE_ARN=arn:aws:iam::123456789012:role/role-name-2",
			},
		},
		{
			Name: "aws_values_use_specifics_if_no_defaults",
			TotalEnv: []string{
				"__SOURCE_TEST__AWS_REGION=us-east-1",
				"AWS_ROLE_ARN=arn:aws:iam::123456789012:role/role-name",
				"__SOURCE_TEST__AWS_ROLE_ARN=arn:aws:iam::123456789012:role/role-name-2",
			},
			FilteredEnv: []string{
				"AWS_REGION=us-east-1",
				"AWS_ROLE_ARN=arn:aws:iam::123456789012:role/role-name-2",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			filteredEnv := filterPluginEnv(tc.TotalEnv, "test", "source")
			sort.Strings(filteredEnv)
			sort.Strings(tc.FilteredEnv)
			require.Equal(t, tc.FilteredEnv, filteredEnv)
		})
	}
}
func readSummaries(t *testing.T, filename string) []syncSummary {
	p, err := os.ReadFile(filename)
	assert.NoError(t, err)

	lines := bytes.Split(p, []byte{'\n'})
	summaries := make([]syncSummary, len(lines))
	for i, line := range lines {
		if len(line) == 0 {
			summaries = slices.Delete(summaries, i, i+1)
			continue
		}
		var v syncSummary
		assert.NoError(t, json.Unmarshal(line, &v))
		summaries[i] = v
	}
	return summaries
}
