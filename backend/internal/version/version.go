// Package version 保存构建时注入的版本信息。
//
// 版本与构建时间由构建命令通过 -ldflags -X 注入；未注入时 Version 为 dev。
package version

var (
	// Version 是构建版本号，形如 v1.4.1；本地开发构建为 dev。
	Version = "dev"
	// BuildTime 是构建时刻（UTC RFC3339）；未注入时为空串。
	BuildTime = ""
)
