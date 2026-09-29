// Package app 是 Wails 绑定层：按领域拆成多个 Service，只做参数校验和转发，
// 业务逻辑在 internal/service。替代 v1 的 gin controller。
//
// 新增 Service 后在 main.go 的 Bind 列表里注册，并运行 `wails generate module`
// 生成 frontend/wailsjs/go/app/*.js 和 models.ts。
package app
