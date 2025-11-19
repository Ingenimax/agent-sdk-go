module deployment_config

go 1.24.5

replace github.com/Ingenimax/agent-sdk-go => ../..

require github.com/Ingenimax/agent-sdk-go v0.0.0-00010101000000-000000000000

require (
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/rs/zerolog v1.34.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
)
