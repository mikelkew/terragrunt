package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/require"
)

const (
	includeDeepFixturePath      = "fixture-include-deep/"
	includeDeepFixtureChildPath = "child"
	includeFixturePath          = "fixture-include/"
	includeShallowFixturePath   = "stage/my-app"
	includeNoMergeFixturePath   = "qa/my-app"
	includeExposeFixturePath    = "fixture-include-expose/"
	includeChildFixturePath     = "child"
	includeMultipleFixturePath  = "fixture-include-multiple/"
	includeRunAllFixturePath    = "fixture-include-runall/"
)

func TestTerragruntWorksWithIncludeLocals(t *testing.T) {
	t.Parallel()

	files, err := os.ReadDir(includeExposeFixturePath)
	require.NoError(t, err)

	testCases := []string{}
	for _, finfo := range files {
		if finfo.IsDir() {
			testCases = append(testCases, finfo.Name())
		}
	}

	for _, testCase := range testCases {
		// Capture range variable to avoid it changing across parallel test runs
		testCase := testCase

		t.Run(filepath.Base(testCase), func(t *testing.T) {
			childPath := filepath.Join(includeExposeFixturePath, testCase, includeChildFixturePath)
			cleanupTerraformFolder(t, childPath)
			runTerragrunt(t, fmt.Sprintf("terragrunt run-all apply -auto-approve --terragrunt-include-external-dependencies --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s", childPath))

			stdout := bytes.Buffer{}
			stderr := bytes.Buffer{}
			err := runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s", childPath), &stdout, &stderr)
			require.NoError(t, err)

			outputs := map[string]TerraformOutput{}
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
			require.Equal(t, "us-west-1-test", outputs["region"].Value.(string))
		})
	}
}

func TestTerragruntWorksWithIncludeShallowMerge(t *testing.T) {
	t.Parallel()

	childPath := util.JoinPath(includeFixturePath, includeShallowFixturePath)
	cleanupTerraformFolder(t, childPath)

	s3BucketName := fmt.Sprintf("terragrunt-test-bucket-%s", strings.ToLower(uniqueId()))
	defer deleteS3Bucket(t, TERRAFORM_REMOTE_STATE_S3_REGION, s3BucketName)

	tmpTerragruntConfigPath := createTmpTerragruntConfigWithParentAndChild(t, includeFixturePath, includeShallowFixturePath, s3BucketName, config.DefaultTerragruntConfigPath, config.DefaultTerragruntConfigPath)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, childPath))
	validateIncludeRemoteStateReflection(t, s3BucketName, includeShallowFixturePath, tmpTerragruntConfigPath, childPath)
}

func TestTerragruntWorksWithIncludeNoMerge(t *testing.T) {
	t.Parallel()

	childPath := util.JoinPath(includeFixturePath, includeNoMergeFixturePath)
	cleanupTerraformFolder(t, childPath)

	// We deliberately pick an s3 bucket name that is invalid, as we don't expect to create this s3 bucket.
	s3BucketName := "__INVALID_NAME__"

	tmpTerragruntConfigPath := createTmpTerragruntConfigWithParentAndChild(t, includeFixturePath, includeNoMergeFixturePath, s3BucketName, config.DefaultTerragruntConfigPath, config.DefaultTerragruntConfigPath)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-config %s --terragrunt-working-dir %s", tmpTerragruntConfigPath, childPath))
	validateIncludeRemoteStateReflection(t, s3BucketName, includeNoMergeFixturePath, tmpTerragruntConfigPath, childPath)
}

func TestTerragruntRunAllModulesThatIncludeRestrictsSet(t *testing.T) {
	t.Parallel()

	rootPath := copyEnvironment(t, includeRunAllFixturePath)
	modulePath := util.JoinPath(rootPath, includeRunAllFixturePath)
	cleanupTerraformFolder(t, modulePath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(
		t,
		fmt.Sprintf(
			"terragrunt run-all plan --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s --terragrunt-modules-that-include alpha.hcl",
			modulePath,
		),
		&stdout,
		&stderr,
	)
	require.NoError(t, err)
	logBufferContentsLineByLine(t, stdout, "stdout")
	logBufferContentsLineByLine(t, stderr, "stderr")

	planOutput := stdout.String()
	require.Contains(t, planOutput, "alpha")
	require.NotContains(t, planOutput, "beta")
	require.NotContains(t, planOutput, "charlie")
}

func TestTerragruntRunAllModulesWithPrefix(t *testing.T) {
	t.Parallel()

	rootPath := copyEnvironment(t, includeRunAllFixturePath)
	modulePath := util.JoinPath(rootPath, includeRunAllFixturePath)
	cleanupTerraformFolder(t, modulePath)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(
		t,
		fmt.Sprintf(
			"terragrunt run-all plan --terragrunt-non-interactive --terragrunt-include-module-prefix --terragrunt-working-dir %s",
			modulePath,
		),
		&stdout,
		&stderr,
	)
	require.NoError(t, err)
	logBufferContentsLineByLine(t, stdout, "stdout")
	logBufferContentsLineByLine(t, stderr, "stderr")

	planOutput := stdout.String()
	require.Contains(t, planOutput, "alpha")
	require.Contains(t, planOutput, "beta")
	require.Contains(t, planOutput, "charlie")

	stdoutLines := strings.Split(planOutput, "\n")
	for _, line := range stdoutLines {
		if strings.Contains(line, "alpha") {
			require.Contains(t, line, includeRunAllFixturePath+"a")
		}
		if strings.Contains(line, "beta") {
			require.Contains(t, line, includeRunAllFixturePath+"b")
		}
		if strings.Contains(line, "charlie") {
			require.Contains(t, line, includeRunAllFixturePath+"c")
		}
	}
}

func TestTerragruntWorksWithIncludeDeepMerge(t *testing.T) {
	t.Parallel()

	childPath := util.JoinPath(includeDeepFixturePath, "child")
	cleanupTerraformFolder(t, childPath)

	runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s", childPath))

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s", childPath), &stdout, &stderr)
	require.NoError(t, err)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))

	require.Equal(t, "mock", outputs["attribute"].Value.(string))
	require.Equal(t, "new val", outputs["new_attribute"].Value.(string))
	require.Equal(t, "old val", outputs["old_attribute"].Value.(string))
	require.Equal(t, []interface{}{"hello", "mock"}, outputs["list_attr"].Value.([]interface{}))
	require.Equal(t, map[string]interface{}{"foo": "bar", "bar": "baz", "test": "new val"}, outputs["map_attr"].Value.(map[string]interface{}))

	require.Equal(
		t,
		map[string]interface{}{
			"attribute":     "mock",
			"new_attribute": "new val",
			"old_attribute": "old val",
			"list_attr":     []interface{}{"hello", "mock"},
			"map_attr": map[string]interface{}{
				"foo": "bar",
				"bar": "baz",
			},
		},
		outputs["dep_out"].Value.(map[string]interface{}),
	)
}

func TestTerragruntWorksWithMultipleInclude(t *testing.T) {
	t.Parallel()

	files, err := os.ReadDir(includeMultipleFixturePath)
	require.NoError(t, err)

	testCases := []string{}
	for _, finfo := range files {
		if finfo.IsDir() && filepath.Base(finfo.Name()) != "modules" {
			testCases = append(testCases, finfo.Name())
		}
	}

	for _, testCase := range testCases {
		// Capture range variable to avoid it changing across parallel test runs
		testCase := testCase

		t.Run(filepath.Base(testCase), func(t *testing.T) {
			t.Parallel()

			childPath := filepath.Join(includeMultipleFixturePath, testCase, includeDeepFixtureChildPath)
			cleanupTerraformFolder(t, childPath)
			runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s", childPath))

			stdout := bytes.Buffer{}
			stderr := bytes.Buffer{}
			err := runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-working-dir %s", childPath), &stdout, &stderr)
			require.NoError(t, err)

			outputs := map[string]TerraformOutput{}
			require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
			validateMultipleIncludeTestOutput(t, outputs)
		})
	}
}

func validateMultipleIncludeTestOutput(t *testing.T, outputs map[string]TerraformOutput) {
	require.Equal(t, "mock", outputs["attribute"].Value.(string))
	require.Equal(t, "new val", outputs["new_attribute"].Value.(string))
	require.Equal(t, "old val", outputs["old_attribute"].Value.(string))
	require.Equal(t, []interface{}{"hello", "mock", "foo"}, outputs["list_attr"].Value.([]interface{}))
	require.Equal(t, map[string]interface{}{"foo": "bar", "bar": "baz", "test": "new val"}, outputs["map_attr"].Value.(map[string]interface{}))

	require.Equal(
		t,
		map[string]interface{}{
			"attribute":     "mock",
			"new_attribute": "new val",
			"old_attribute": "old val",
			"list_attr":     []interface{}{"hello", "mock", "foo"},
			"map_attr": map[string]interface{}{
				"foo": "bar",
				"bar": "baz",
			},
		},
		outputs["dep_out"].Value.(map[string]interface{}),
	)
}

func validateIncludeRemoteStateReflection(t *testing.T, s3BucketName string, keyPath string, configPath string, workingDir string) {
	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt output -no-color -json --terragrunt-non-interactive --terragrunt-log-level debug --terragrunt-config %s --terragrunt-working-dir %s", configPath, workingDir), &stdout, &stderr)
	require.NoError(t, err)

	outputs := map[string]TerraformOutput{}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &outputs))
	remoteStateOut := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(outputs["reflect"].Value.(string)), &remoteStateOut))
	require.Equal(
		t,
		map[string]interface{}{
			"backend":                         "s3",
			"disable_init":                    false,
			"disable_dependency_optimization": false,
			"generate":                        nil,
			"config": map[string]interface{}{
				"encrypt": true,
				"bucket":  s3BucketName,
				"key":     keyPath + "/terraform.tfstate",
				"region":  "us-west-2",
			},
		},
		remoteStateOut,
	)
}
