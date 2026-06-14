# Ports & Adapters (Consumer-Owned Interfaces)

**Status:** OFFICIAL STANDARD. **Companion of** `ARCHITECTURE_STANDARD.md`.
Defines how modules depend on each other through interfaces — and where those
interfaces live. The governing principle: **the consumer owns the port.**

---

## 1. The rule set

1. **Consumer owns the interface.** The module that *needs* a capability declares the
   narrow interface it needs, in its own package. It does not import the provider.
2. **Adapters implement the interface.** The provider module (or a thin adapter) has a
   type that satisfies the consumer's interface.
3. **Composition root wires.** `cmd/scraper/main.go` (and `cmd/worker/main.go`)
   construct the concrete provider and inject it into the consumer. Only `main` knows
   both sides.
4. **No god-package.** There is NO `internal/contracts` (or `internal/interfaces`)
   dumping ground created solely to dodge an import cycle. If two modules seem to need
   a shared interface bucket, that is a smell that the consumer hasn't been identified.

## 2. Why "consumer-owned" (the outbound/executor example)

A Facebook comment is *executed* by Facebook-specific code, but the thing that decides
*when* to execute and tracks the attempt is **outbound** (vertical-neutral). So:

- **outbound is the consumer** of an "executor". It defines the port:

  ```go
  // package outbound  (the consumer)
  // ActionExecutor performs a planned outbound action and reports the outcome.
  // outbound owns this interface because outbound consumes executors; it must not
  // import services/facebook to call one.
  type ActionExecutor interface {
      Execute(ctx context.Context, action PlannedAction) (Outcome, error)
  }
  ```

- **services/facebook is the adapter.** It implements `outbound.ActionExecutor` with
  FB selectors/connector calls. facebook imports outbound (downward) to satisfy the
  port; outbound never imports facebook.

- **facebook workflows consume a narrow planner port.** When a Facebook workflow needs
  to *queue* an action, it depends on a small interface IT owns:

  ```go
  // package facebook  (the consumer of "queue this action")
  type OutboundPlanner interface {
      QueueComment(ctx context.Context, cmd CommentCommand) (QueueResult, error)
  }
  ```

  outbound provides a type satisfying it; `main` injects it.

- **Composition root** wires both directions:

  ```go
  // cmd/scraper/main.go
  fbExecutor := facebook.NewExecutor(connectors, sessions)   // adapter
  outboundSvc := outbound.New(store, fbExecutor)             // consumer gets its port
  fbWorkflows := facebook.NewWorkflows(outboundSvc)          // consumer gets its planner
  ```

This is the inversion that lets Taobao reuse outbound unchanged: `services/taobao`
implements the same `outbound.ActionExecutor`; outbound doesn't grow a Taobao import.

## 3. The Copilot / ActionHandler case (current → target)

**Today:** `internal/ai/agent.go` holds `*store.Store` and an `ActionHandler
func(action string, args map[string]any) (string, error)` injected from
`cmd/scraper`. This works but the driver imports the store and the handler is an
untyped `map[string]any` bag.

**Target:** the Copilot *driver* depends on a typed **application command port** it
owns, with no store import:

```go
// package copilot (the consumer / inbound driver)
type CommandBus interface {
    Dispatch(ctx context.Context, cmd Command) (Result, error)  // Command is a typed union
}
```

`services/facebook` provides handlers for the FB commands; `main` wires a dispatcher.
The driver translates NL → `Command` and calls `Dispatch`. It never touches the DB.

## 4. Where ports live (directory rule)

| Interface | Lives in (consumer package) | Implemented by |
|---|---|---|
| `outbound.ActionExecutor` | `internal/store/outbound` (or `internal/outbound`) | `services/facebook`, future `services/taobao` |
| `facebook.OutboundPlanner` | `services/facebook` | outbound |
| `copilot.CommandBus` | `drivers/copilot` (`internal/ai` driver side) | application dispatcher in `main`/services |
| `notifications.Sink` | `internal/telegram/control` consumer side | telegram client adapter |
| `ai` ports (classify/generate) | the **service** that calls AI owns the port | `internal/ai` generators implement it |

Note the asymmetry on AI: because `internal/ai` is **pure** and must not depend on
callers, the *service* declares the narrow port it needs (e.g. `CommentRepairer`) and
the `ai` type satisfies it structurally. AI stays import-free.

## 5. Anti-patterns (forbidden)

- ❌ `internal/contracts` holding every interface "to avoid cycles". Cycles are avoided
  by putting the interface with its consumer, not in a shared bucket.
- ❌ Provider defining the interface and the consumer importing the provider to get it
  (that re-couples consumer→provider).
- ❌ `map[string]any` as the cross-module contract for new code. Use typed commands.
  (The existing `ActionHandler(map[string]any)` is legacy; new ports are typed.)
- ❌ A consumer reaching around its port to the concrete type (`x.(*facebook.Executor)`).
- ❌ Wiring dependencies anywhere but the composition root (`main`). No global
  singletons, no `init()` registration of cross-module providers.

## 6. Composition root responsibilities

`cmd/scraper/main.go` and `cmd/worker/main.go` are the ONLY places that:

- construct concrete adapters (store, connectors, executors, AI generators);
- inject them into consumers (services, drivers);
- wire the event relay + process managers;
- own the lazy-getter setters (e.g. `SetUniversalClassifier`) for things resolved
  after route registration.

Everything else receives its dependencies; nothing else news-up a cross-module
provider.
