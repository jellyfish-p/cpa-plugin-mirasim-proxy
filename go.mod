module github.com/router-for-me/cliproxy-plugin-mirasim

go 1.26.0

require (
	github.com/gorilla/websocket v1.5.3
	github.com/router-for-me/CLIProxyAPI/v7 v7.0.0
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/router-for-me/CLIProxyAPI/v7 => ../../CLIProxyAPI
