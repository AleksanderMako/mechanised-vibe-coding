package configpkg

// Config exercises every tag form the contract describes.
type Config struct {
	Port     int    `arg:"env"`
	Host     string `arg:"--host,env:APP_HOST,required"`
	Mode     string `arg:"env:APP_MODE,required" default:"dev"`
	Timeout  string `arg:"env:APP_TIMEOUT" default:"30s"`
	Verbose  bool   `arg:"-v,--verbose"`
	Untagged string
	Secret   string   `arg:"-"`
	Hidden   Database `arg:"-"`
	Database Database
	Logging
	Primary  string `arg:"env:SHARED_URL"`
	Fallback string `arg:"env:SHARED_URL,required"`
}

// NotAStruct is not a struct, so asking Vars for it is an error.
type NotAStruct string

func helper() {
	// LocalConfig is not top level, so asking Vars for it is an error.
	type LocalConfig struct {
		Name string `arg:"env:LOCAL_NAME"`
	}
	_ = LocalConfig{}
}
