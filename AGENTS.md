# AGENTS.md

JIMM (Juju Intelligent Model Manager) is a Go service that manages multiple Juju controllers as part of JAAS.

## Repository layout

- [`cmd/jimmsrv`](./cmd/jimmsrv): JIMM server entry point.
- [`cmd/jaas`](./cmd/jaas): JAAS plugin for the Juju CLI.
- [`internal/`](./internal): Server implementation and tests; [`pkg/`](./pkg) contains reusable API and naming packages.
- [`docs/`](./docs): User documentation. For landing pages, see [the landing page standards](./AGENTS.documentation-landing-pages.md).

## Development workflow

- Keep changes focused, follow nearby conventions, and run `gofmt` on changed Go files. Use `testing.T` for new Go tests and `quicktest` for assertions.
- Run the narrowest relevant tests. Before running service-backed or end-to-end tests, follow the [spin-up-test-env skill](./.github/skills/spin-up-test-env/SKILL.md).
- Prefer adding tests to the [`testing/`](./testing) folder where tests are written in Go and reserve [`tests/`](./tests) for exceptional cases where we need to use bash.
- Run `make lint` for Go linting. If a change affects generated code or JAAS CLI reference docs, run `make generate` and include the relevant generated updates.

## Things to keep in mind

- JIMM aims to be compatible with multiple major versions of Juju controllers.
- JIMM has a single Juju client that is able to talk with Juju v4 and v3 backing controllers.
- JIMM can receive requests from v3 clients, in which case it needs to strictly target v3 backing controller.
- JIMM can receive requests from v4 client, in which case it can target all backing controllers.
- JIMM has 2 types of requests, model requests which are proxied to the backing controller, and controller facade requests which are implemented by JIMM. The implementation must generally match Juju controller behavior with the additional features of JIMM.
- JIMM needs to implement facade versions implemented by Juju v3 and Juju v4 controllers.
