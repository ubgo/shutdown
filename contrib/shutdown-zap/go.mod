module github.com/ubgo/shutdown/contrib/shutdown-zap

go 1.24

require (
	github.com/ubgo/shutdown v0.2.0
	go.uber.org/zap v1.27.0
)

require go.uber.org/multierr v1.10.0 // indirect

replace github.com/ubgo/shutdown => ../..
