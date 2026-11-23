package configpkg

// TestOnlyConfig sits in a test file, which Vars must ignore.
type TestOnlyConfig struct {
	Token string `arg:"env:TEST_ONLY_TOKEN"`
}
