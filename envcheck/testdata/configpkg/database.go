package configpkg

// Database lives in a second file to show the whole package is parsed.
type Database struct {
	Host     string `arg:"env:DB_HOST,required"`
	Password string `arg:"env:DB_PASSWORD"`
	Ignored  string `arg:"-"`
}

// Logging is embedded in Config.
type Logging struct {
	Level string `arg:"env:LOG_LEVEL" default:"info"`
}
