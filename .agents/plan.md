# Plan: Refactor `internal/server/` theo subfolder, sau đó fix deploy.yml + version bug

## Context

User đang gặp 3 vấn đề quan hệ chặt với nhau:

1. **Bug version extension**: `local-connector-extension/manifest.json` đã được update lên `0.2.1` (commit `ff72724`), nhưng khi user tải zip từ route beta thật (`/api/system/extension-beta-package`) trên server production thì vẫn nhận về version `0.2.0`.

2. **GitHub Actions `deploy.yml` fail** với lỗi:
   > `Invalid workflow file: .github/workflows/deploy.yml#L365 — You have an error in your yaml syntax on line 365`
   
   Vì workflow fail ngay từ YAML parse stage, KHÔNG step nào chạy → zip mới (0.2.1) chưa bao giờ được upload lên server → user vẫn thấy 0.2.0. **Bug version chính là hệ quả của deploy.yml fail.**

3. **Codebase structure không vững**: file lớn quá (`api.go` 1247 lines, `workspace_handlers.go` 1114, `agent_handlers.go` 828); flat layout 26 file cùng cấp trong `internal/server/`; tên file mơ hồ (`helpers.go`, `superadmin_extra.go`, `account_guard.go`); 3 file connector + 6 file auth nằm rải rác. User muốn dọn cấu trúc trước rồi mới fix bug để codebase bền vững.

User đã chọn (qua AskUserQuestion):
- **Refactor scope**: tổ chức lại `internal/server/` thành subfolder theo domain
- **Sequencing**: refactor xong rồi mới fix version + deploy
- **Bug observation**: tải zip thật trên server vẫn thấy 0.2.0

Đầu ra mong muốn: codebase có subpackage rõ ràng theo domain, deploy.yml parse thành công, deploy chạy hết lượt và route beta trả về zip 0.2.1.

---

## Findings (từ Phase 1 exploration)

### Codebase audit
- `internal/server/api.go` (1247 lines): vừa là router setup, vừa middleware wiring, vừa chứa ~39 handler nội tuyến (lead/post/group/account CRUD), vừa wire 157 route. Phần nhiều handler đã được tách ra `*_handlers.go` nhưng không nhất quán.
- 26 file `.go` flat trong `internal/server/`, không subdirectory.
- File lớn ngoài api.go: `workspace_handlers.go` (1114), `agent_handlers.go` (828).
- File tên mơ hồ: `helpers.go`, `cdp_helpers.go`, `superadmin_extra.go`, `system_handlers.go`, `account_guard.go`, `input_limits.go`, `identity_sync.go`.
- Domain bị phân mảnh: connector logic ở 3 file (`agent_handlers.go`, `data_connector_handlers.go`, `local_connector_handlers.go`); auth ở 6 file.

### Deploy.yml YAML error tại line 365
Đoạn code lỗi (`.github/workflows/deploy.yml:155, 364-369, 372-377`):

```yaml
script: |
              # ... bash code ở indent 12 ...
              INSTALLED_BETA_VERSION=$(sudo python3 - <<PY
import json, zipfile                    # ← line 365, ở cột 0!
with zipfile.ZipFile('$BETA_VERIFY_PATH') as z:
    print(json.loads(z.read('manifest.json'))['version'])
PY
)
              # ...
              LIVE_BETA_VERSION=$(python3 - <<'PY'
import json, zipfile                    # ← cùng vấn đề ở line 373
with zipfile.ZipFile('/tmp/live-beta.zip') as z:
    print(json.loads(z.read('manifest.json'))['version'])
PY
)
```

**Nguyên nhân**: `script: |` là YAML literal block scalar được nest trong `with:` ở indent 12. Mọi dòng trong block phải có indent ≥ 12. Heredoc body Python ở cột 0 phá vỡ block, YAML parser báo syntax error tại line 365 và **abort toàn bộ workflow**. Không step nào (build, test, scp, ssh) chạy.

Cùng lỗi xảy ra với heredoc thứ 2 (line 372-377) nhưng parser đã abort ở error đầu nên user chỉ thấy báo lỗi line 365.

### Tại sao bug version 0.2.0 còn dính
- `manifest.json` trên `main` đã là `0.2.1` ✓
- `internal/server/system_handlers.go:10-25` (`serveExtensionBetaPackage`) KHÔNG cache trong Go — đọc trực tiếp từ disk với `Cache-Control: no-store`.
- `chromeExtensionBetaPackagePath()` ở `internal/server/api.go:94-100` trả về `CHROME_EXTENSION_BETA_PACKAGE_PATH` env (mặc định `/opt/thg-scraper/releases/thg-chrome-extension.zip`).
- Kết luận: file `.zip` trên server bị stale ở 0.2.0 vì `deploy.yml` chưa bao giờ chạy thành công kể từ commit `ff72724`. Fix YAML → deploy chạy → zip được overwrite → route trả về 0.2.1.

---

## Stage A — Refactor `internal/server/` theo subfolder domain

### Ràng buộc cần biết trước

Go package = directory. Hiện tất cả handler đang là method trên `*server.Server` (struct ở `api.go:103-115`). Khi tách thành subpackage, KHÔNG thể giữ nguyên kiểu method receiver vì:
- Subpackage import `server` thì OK, nhưng `server.Server` import ngược subpackage → **circular dependency**.
- Cách giải: handler trong subpackage chuyển thành function nhận dependency tường minh (DB, jobStore, agent, cfg, wsHub, workspace manager) thay vì receiver `*Server`. Dependency được pass vào hàm `Routes(...)` của mỗi subpackage khi router gốc gọi.

### Cấu trúc đích

```
internal/server/
├── server.go              ← Server struct, New(), SetX() setters (tách từ api.go)
├── router.go              ← gắn middleware global, gọi Routes() của từng subpackage
├── deps.go                ← Deps struct gom DB/jobStore/agent/wsHub/workspace/cfg để pass cho subpackage
├── middleware/
│   ├── auth.go            ← tenantReady, adminOnly, role guards (tách từ api.go:300-330)
│   ├── ratelimit.go       ← rate limit configs theo class endpoint
│   └── input_limits.go    ← (đổi tên giữ nguyên content)
├── auth/                  ← package auth
│   ├── routes.go          ← Routes(group, deps)
│   ├── handlers.go        ← login/signup/refresh/changePassword (từ auth_handlers.go)
│   ├── google.go          ← (từ google_auth.go)
│   ├── login_session.go   ← Chrome login session (từ login_handlers.go)
│   ├── onboarding.go      ← (từ onboarding_handlers.go)
│   ├── provisioning.go    ← org provisioning on signup (từ auth_provisioning.go)
│   └── invite.go          ← (từ invite_email.go)
├── workspace/             ← package workspace
│   ├── routes.go
│   ├── handlers.go        ← split workspace_handlers.go (1114 lines) thành public handlers
│   ├── watchers.go        ← internal browser-session state watchers (phần private của workspace_handlers.go)
│   ├── screen_proxy.go    ← (giữ nguyên)
│   └── vnc_proxy.go       ← (giữ nguyên)
├── agent/                 ← package agent (gom 3 connector file)
│   ├── routes.go
│   ├── heartbeat.go       ← agentHeartbeat, chromeStatus (từ agent_handlers.go)
│   ├── crawl.go           ← agentConnectorCrawlResult (từ agent_handlers.go)
│   ├── commands.go        ← agentCommands (từ agent_handlers.go)
│   ├── data_connector.go  ← (từ data_connector_handlers.go, đổi tên gọn)
│   ├── local_connector.go ← (từ local_connector_handlers.go, đổi tên gọn)
│   ├── ws_hub.go          ← (từ ws_hub.go) — chỉ hub Chrome Extension nên ở agent/
│   └── account_guard.go   ← (từ account_guard.go, đặt cạnh nơi dùng)
├── org/                   ← package org
│   ├── routes.go
│   ├── handlers.go        ← registerOrg/getMyOrg/updateOrg/createOrgUser (từ organization_handlers.go)
│   ├── users.go           ← user CRUD admin (từ api.go:312-318 inline handlers)
│   ├── superadmin.go      ← (từ superadmin_extra.go, đổi tên cho rõ)
│   └── identity.go        ← (từ identity_sync.go, đổi tên cho gọn)
├── system/                ← package system
│   ├── routes.go
│   ├── extension.go       ← serveExtensionBetaPackage + /api/system/info (từ system_handlers.go + api.go:172-186)
│   ├── logs.go            ← (từ log_handlers.go)
│   └── notifications.go   ← (từ notifications.go)
├── skills/                ← package skills (cho Workspace Skill Designer per CLAUDE.md)
│   ├── routes.go
│   └── handlers.go        ← (từ skills_handlers.go)
├── autoflow/              ← package autoflow
│   ├── routes.go
│   ├── kpi.go             ← (từ autoflow_handlers.go phần KPI)
│   ├── files.go           ← (file management)
│   ├── threads.go
│   └── data_sources.go
├── leads/                 ← package leads (gom CRUD inline trong api.go)
│   ├── routes.go
│   └── handlers.go        ← getLeads, deleteAllLeads, niches, posts, groups (từ api.go inline ~line 360-500)
└── crawl/
    └── intent.go          ← (từ crawl_intent_handlers.go)
```

### Bước thực hiện refactor (theo thứ tự, mỗi bước build/test trước khi sang bước sau)

**A1. Extract `Server` struct, dependency holder.**
- Tạo `internal/server/server.go`: copy `Server` struct + `New()` + `SetSessionRegistry/SetAgentHandler/SetUniversalClassifier` từ `api.go:103-162`.
- Tạo `internal/server/deps.go`: struct `Deps { DB, JobStore, Agent, WSHub, Workspace, Cfg, JWTSecret, AIClass }`. Phương thức `(s *Server) Deps() *Deps` xuất ra cho subpackage.
- Trong `api.go`, chỉ giữ lại: `package server` declaration, helper `chromeExtensionBetaInfo()`, `chromeExtensionStoreInfo()`, `chromeExtensionBetaPackagePath()`, `envFlagEnabled()`. Phần còn lại sẽ rỗng dần khi route được di dời.
- `go build ./...` xác nhận không gãy.

**A2. Tạo `middleware/` subpackage.**
- File `auth.go`: di chuyển `tenantReady` (api.go:300-330), `adminOnly`, role guards. Export `RequireTenant(jwtSecret string)`, `RequireAdmin()`, `RequirePlatformRole(...)`.
- File `ratelimit.go`: chuyển các rate limit middleware config từ `api.go` (search `limiter.New`).
- File `input_limits.go`: di chuyển nguyên `internal/server/input_limits.go`.
- `go build ./...` + `go vet ./...`.

**A3. Tạo từng subpackage, di dời 1 domain mỗi lần** (theo thứ tự ít rủi ro → nhiều rủi ro):

  Order: `system` → `leads` → `org` → `skills` → `autoflow` → `auth` → `agent` → `workspace`.

  Mỗi domain:
  1. Tạo thư mục + `package <name>`.
  2. Move file handler tương ứng. Đổi method `func (s *Server) handlerName(c *fiber.Ctx)` thành `func handlerName(deps *server.Deps, c *fiber.Ctx)` HOẶC dùng closure: `func handlerName(deps *server.Deps) fiber.Handler { return func(c *fiber.Ctx) error { ... } }`. **Chọn closure pattern** vì compatible với Fiber Get/Post signature.
  3. Tạo `routes.go`: `func Routes(group fiber.Router, deps *server.Deps) { group.Get("/leads", handlerName(deps)); ... }`.
  4. Trong `api.go` (sau này thành `router.go`): import subpackage, gọi `leads.Routes(r, s.Deps())`.
  5. Xóa route registration cũ + handler cũ trong api.go.
  6. `go build ./... && go test ./internal/server/... && go vet ./...`.
  7. Commit nhỏ riêng cho domain đó.

**A4. Đổi tên file mơ hồ trong khi di dời** (đã ánh xạ trong cấu trúc đích ở trên):
- `superadmin_extra.go` → `org/superadmin.go`
- `system_handlers.go` → `system/extension.go`
- `account_guard.go` → `agent/account_guard.go`
- `identity_sync.go` → `org/identity.go`
- `helpers.go` (`internal/store/helpers.go`) → tách: `dedup.go` (hashing), `sqlite_time.go` (time parsing), `errors.go` (busy-error detection). KHÔNG bắt buộc phải làm trong scope này — đánh dấu follow-up nếu thời gian gấp.
- `cdp_helpers.go` → `internal/runtime/cdp_targets.go`.

**A5. Tách `workspace_handlers.go` (1114 lines)** — bước rủi ro nhất, làm cuối:
- Đọc kỹ file, phân loại từng function: handler HTTP/WS công khai vs helper riêng (state watcher, mutator). Helper riêng → `watchers.go`. Public handler → `handlers.go`.
- Test workspace flows thủ công sau khi tách: list/create/start/stop workspace, navigate, screenshot.

**A6. Cuối refactor: api.go → router.go**
- Đổi tên `api.go` thành `router.go`.
- Xác nhận file < 200 lines, chỉ còn: app config, global middleware, gọi `<domain>.Routes(...)`.

**A7. Verification toàn phase A**:
```powershell
go build ./...
go vet ./...
go test ./... -timeout 60s
npm --prefix frontend run build
```

---

## Stage B — Fix `deploy.yml` line 365 YAML syntax

Sửa cả 2 heredoc Python để body không phá vỡ YAML block scalar. Thay bằng `python3 -c` inline (đơn giản nhất, không cần heredoc):

**File**: `.github/workflows/deploy.yml`

**Đoạn 1 (line 364-369)** — thay:
```yaml
              INSTALLED_BETA_VERSION=$(sudo python3 - <<PY
import json, zipfile
with zipfile.ZipFile('$BETA_VERIFY_PATH') as z:
    print(json.loads(z.read('manifest.json'))['version'])
PY
)
```
bằng:
```yaml
              INSTALLED_BETA_VERSION=$(sudo python3 -c "import json, zipfile; print(json.loads(zipfile.ZipFile('$BETA_VERIFY_PATH').read('manifest.json'))['version'])")
```

**Đoạn 2 (line 372-377)** — thay:
```yaml
              LIVE_BETA_VERSION=$(python3 - <<'PY'
import json, zipfile
with zipfile.ZipFile('/tmp/live-beta.zip') as z:
    print(json.loads(z.read('manifest.json'))['version'])
PY
)
```
bằng:
```yaml
              LIVE_BETA_VERSION=$(python3 -c "import json, zipfile; print(json.loads(zipfile.ZipFile('/tmp/live-beta.zip').read('manifest.json'))['version'])")
```

Lưu ý chi tiết:
- `$BETA_VERIFY_PATH` ở đoạn 1 vẫn được bash expand vì dùng double-quote ngoài.
- `/tmp/live-beta.zip` ở đoạn 2 là literal path nên không cần expand.
- Cả 2 dùng `python3 -c "..."` nên không còn heredoc → không còn lệch indent → YAML parse OK.

**Validate trước khi commit**:
```powershell
# Cài actionlint nếu chưa có (https://github.com/rhysd/actionlint)
actionlint .github/workflows/deploy.yml
# Hoặc dùng python yaml lint:
python -c "import yaml; yaml.safe_load(open('.github/workflows/deploy.yml'))"
```

---

## Stage C — Verify deploy success + version 0.2.1

1. Push commit fix YAML lên `main`.
2. Quan sát workflow `CI` chạy xong (đã pass sẵn) → trigger `Deploy to Server`.
3. Theo dõi từng step trong tab Actions:
   - "Build THG Chrome Extension Web Store package" → kỳ vọng zip được tạo ở `thg-extension-store-package/thg-chrome-extension.zip`.
   - "Install and restart" → kỳ vọng log dòng `Chrome Extension beta package installed at /opt/thg-scraper/releases/thg-chrome-extension.zip`.
   - "Beta route verified: version 0.2.1" → đây là dòng confirm cuối.
4. Verify thủ công từ máy local:
   ```powershell
   curl -fsSL "https://sale.thgfulfill.com/api/system/extension-beta-package" -o live-beta.zip
   python -c "import json, zipfile; print(json.loads(zipfile.ZipFile('live-beta.zip').read('manifest.json'))['version'])"
   # Kỳ vọng output: 0.2.1
   ```
5. Trên trang `/extension-beta`, click nút download và mở zip — `manifest.json` phải hiện `"version": "0.2.1"`.

---

## Critical files

**Refactor (Stage A)** — đụng tới hoặc tạo mới:
- [internal/server/api.go](internal/server/api.go) — sẽ co lại còn ~150 lines, đổi tên thành `router.go`
- [internal/server/workspace_handlers.go](internal/server/workspace_handlers.go) — split sang `internal/server/workspace/`
- [internal/server/agent_handlers.go](internal/server/agent_handlers.go) — split sang `internal/server/agent/`
- [internal/server/auth_handlers.go](internal/server/auth_handlers.go), [auth_provisioning.go](internal/server/auth_provisioning.go), [google_auth.go](internal/server/google_auth.go), [login_handlers.go](internal/server/login_handlers.go), [onboarding_handlers.go](internal/server/onboarding_handlers.go), [invite_email.go](internal/server/invite_email.go) — gom vào `internal/server/auth/`
- [internal/server/data_connector_handlers.go](internal/server/data_connector_handlers.go), [local_connector_handlers.go](internal/server/local_connector_handlers.go), [ws_hub.go](internal/server/ws_hub.go), [account_guard.go](internal/server/account_guard.go) — gom vào `internal/server/agent/`
- [internal/server/organization_handlers.go](internal/server/organization_handlers.go), [superadmin_extra.go](internal/server/superadmin_extra.go), [identity_sync.go](internal/server/identity_sync.go) — gom vào `internal/server/org/`
- [internal/server/system_handlers.go](internal/server/system_handlers.go), [log_handlers.go](internal/server/log_handlers.go), [notifications.go](internal/server/notifications.go) — gom vào `internal/server/system/`
- [internal/server/skills_handlers.go](internal/server/skills_handlers.go), [autoflow_handlers.go](internal/server/autoflow_handlers.go), [crawl_intent_handlers.go](internal/server/crawl_intent_handlers.go), [screen_proxy.go](internal/server/screen_proxy.go), [vnc_proxy.go](internal/server/vnc_proxy.go), [input_limits.go](internal/server/input_limits.go) — di dời theo bảng cấu trúc đích.

**Bug fix (Stage B)**:
- [.github/workflows/deploy.yml](.github/workflows/deploy.yml) — sửa line 364-369 và 372-377

**Không đụng** (đã đúng):
- [local-connector-extension/manifest.json](local-connector-extension/manifest.json) — đã là `0.2.1` ✓
- [scripts/build-chrome-extension.sh](scripts/build-chrome-extension.sh) — package đúng, không cần sửa

---

## Reuse / pattern hiện có

- `Server` struct ở [internal/server/api.go:103-115](internal/server/api.go#L103-L115) — sẽ trở thành dependency container chia sẻ.
- `chromeExtensionBetaPackagePath()` ở [internal/server/api.go:94-100](internal/server/api.go#L94-L100) — tái dùng nguyên trong `system/extension.go`.
- `serveExtensionBetaPackage` ở [internal/server/system_handlers.go:10-25](internal/server/system_handlers.go#L10-L25) — đã đúng (đọc disk + `Cache-Control: no-store`), chỉ chuyển package.
- Pattern `app.Get("/api/system/extension-beta-package", s.serveExtensionBetaPackage)` ở [internal/server/api.go:186](internal/server/api.go#L186) — sẽ trở thành `system.Routes(app, s.Deps())` trong `router.go`.

---

## Verification (toàn cục)

Sau khi xong cả 3 stage:

```powershell
# 1. Build & test backend
go build ./...
go vet ./...
go test ./... -timeout 60s

# 2. Build frontend
npm --prefix frontend run build

# 3. Validate workflow YAML
python -c "import yaml; yaml.safe_load(open('.github/workflows/deploy.yml')); print('OK')"

# 4. Sau khi push, kiểm tra GitHub Actions:
#    - CI workflow: pass
#    - Deploy workflow: pass tới step "Beta route verified: version 0.2.1"

# 5. Verify production beta route
curl -fsSL "https://sale.thgfulfill.com/api/system/extension-beta-package" -o live-beta.zip
python -c "import json, zipfile; print(json.loads(zipfile.ZipFile('live-beta.zip').read('manifest.json'))['version'])"
# Kỳ vọng: 0.2.1
```

Smoke-test thủ công các route đã refactor (qua frontend hoặc curl với JWT):
- `POST /api/auth/login` (auth subpackage)
- `GET /api/leads` (leads subpackage)
- `POST /api/workspaces` + `POST /api/workspaces/:id/start` (workspace subpackage)
- `GET /api/agent/heartbeat` (agent subpackage — qua extension thật)
- `GET /api/system/info` + `GET /api/system/extension-beta-package` (system subpackage)
- `GET /api/users` admin (org subpackage)

---

## Rủi ro & mitigation

- **Refactor kéo dài, dễ vỡ**: Stage A đòi hỏi ~10 commit nhỏ (mỗi domain 1 commit). Build + test sau từng bước A1→A6. Nếu bí, có thể dừng ở A4 (đổi tên + tách file trong `package server` flat) và để A5 cho follow-up — nhưng user đã chọn subfolder nên cố gắng đi tới cùng.
- **`workspace_handlers.go` 1114 lines** là điểm rủi ro cao nhất (nhiều state watcher tương tác browser thật). Để CUỐI cùng (A5), test thủ công workspace flow trước khi merge.
- **Bug fix Stage B đơn giản nhưng phụ thuộc Stage C để verify**. Nếu sau khi fix YAML mà deploy vẫn fail ở step khác (ví dụ `Run Go tests` do refactor làm vỡ test), phải debug step đó trước khi xác nhận version 0.2.1 lên server.
- **Không có test e2e cho beta route** trong codebase — verification phải làm thủ công bằng curl trên production sau deploy.

---

## Out of scope (follow-up)

- Tách `internal/store/store.go` (691) và `internal/store/agent_tokens.go` (646), `internal/store/outbound.go` (609) — file lớn nhưng không phải mối lo trước mắt của user.
- Tách `internal/telegram/bot.go` (648), `internal/ai/agent_brain.go` (568), `internal/ai/msggen.go` (556) — domain riêng, để khi cần.
- Đổi tên `internal/store/helpers.go` thành `dedup.go` + `sqlite_time.go` + `errors.go` — đánh dấu nhưng không bắt buộc trong scope này.
- Thiết lập actionlint trong CI để bắt YAML error sớm — đề xuất sau khi xong stage hiện tại.
