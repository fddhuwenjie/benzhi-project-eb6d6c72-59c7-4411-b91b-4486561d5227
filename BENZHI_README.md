# BENZHI_README

## 项目说明
- 项目：benzhi-project-eb6d6c72-59c7-4411-b91b-4486561d5227
- 项目用途：已完整实现公共档案去标识审核服务，包含敏感检测、人工遮蔽决策、独立复核、退回重做、批准冻结、确定性发布及可核验审计链。
- Go 工具链：`golang:1.22`
- 前端工具链：无

## 项目描述
- 项目名称：公共档案去标识审核服务
- 项目概述：一个面向公共档案开放审核人员的 Go HTTP 服务，将待开放档案从材料受理、敏感片段检测、人工去标识、独立复核推进到批准发布，并保留可核验的状态与审计证据。项目按 standard 档位规划，目标不少于 2000 行真实生产 Go 代码和 20 个生产 Go 文件；项目根目录包含简体中文 README.md，说明用途、标准构建、运行和测试方式。
- 核心工作流：审核员创建档案开放案件并登记材料摘要，服务检测潜在敏感片段；审核员逐项确认遮蔽方案后提交独立复核，复核员可退回修改或批准，批准后的案件生成发布清单并进入不可再编辑的已发布状态。
- 对外接口：提供版本化 HTTP JSON API，覆盖案件创建、敏感检测、遮蔽决策、提交复核、退回或批准及发布清单查询；服务支持 -addr=127.0.0.1:<port>，也支持将仅含端口号的 PORT 绑定为 127.0.0.1:<PORT>，默认监听 127.0.0.1:19081，且绝不默认绑定 0.0.0.0、8080、80 或 3000。

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/server -selftest -addr=127.0.0.1:19081
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-eb6d6c72-59c7-4411-b91b-4486561d5227-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-eb6d6c72-59c7-4411-b91b-4486561d5227-arm64 linux/arm64
docker run -it benzhi-project-eb6d6c72-59c7-4411-b91b-4486561d5227-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/server -selftest -addr=127.0.0.1:19081`
