# Load Testing Platform Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the tested foundation for a Go backend API and React frontend console so later Agent, Executor, Scenario, Profiling, and Report work can be added with TDD.

**Architecture:** This plan creates the first vertical slice only: a Go API server with a tested health endpoint, a React/Vite console shell with tested navigation, and a typed frontend health API client. The platform design is intentionally decomposed into smaller implementation plans because the full spec spans several independent subsystems.

**Tech Stack:** Go, chi, React, TypeScript, Vite, Vitest, React Testing Library, jsdom.

---

## Scope

This plan covers the repository foundation and test harness.

Included in this plan:

- Backend Go module.
- Backend API router.
- Backend health endpoint.
- Frontend React/Vite app shell.
- Frontend component test setup.
- Frontend typed health API client.
- Frontend tests for visible console shell behavior and API client behavior.

Not included in this plan:

- Target Agent and machine metrics.
- Scenario editor and step execution.
- Executor load model.
- Adapter RPC driver.
- pprof / prof collection.
- Report timeline and artifacts.
- Authentication and project isolation.

Those areas should each get their own implementation plan after this foundation exists.

## TDD Rules For Execution

- Do not write production behavior before a failing test exists.
- For each behavior, run the focused test and confirm it fails for the expected reason.
- Implement only the minimal code needed to make the focused test pass.
- Run the focused test again and confirm it passes.
- Run the relevant package test suite before committing each task.
- Frontend UI behavior must have React Testing Library tests.
- Frontend API behavior must have Vitest tests using a fetch stub only at the network boundary.
- Backend HTTP behavior must have Go `httptest` tests.

## File Structure

Files created by this plan:

```text
backend/
  go.mod
  cmd/server/main.go
  internal/api/router.go
  internal/api/health_test.go

frontend/
  package.json
  index.html
  tsconfig.json
  tsconfig.node.json
  vite.config.ts
  vitest.setup.ts
  src/main.tsx
  src/app/App.tsx
  src/app/App.test.tsx
  src/api/health.ts
  src/api/health.test.ts
```

## Task 1: Create Test Tooling Configuration

**Files:**

- Create: `backend/go.mod`
- Create: `frontend/package.json`
- Create: `frontend/index.html`
- Create: `frontend/tsconfig.json`
- Create: `frontend/tsconfig.node.json`
- Create: `frontend/vite.config.ts`
- Create: `frontend/vitest.setup.ts`

- [ ] **Step 1: Create backend module configuration**

Create `backend/go.mod`:

```go
module all_in_one_testing/backend

go 1.22

require github.com/go-chi/chi/v5 v5.2.3
```

- [ ] **Step 2: Create frontend package configuration**

Create `frontend/package.json`:

```json
{
  "name": "all-in-one-testing-frontend",
  "version": "0.1.0",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "test": "vitest",
    "test:run": "vitest run"
  },
  "dependencies": {
    "@ant-design/icons": "^5.6.1",
    "antd": "^5.26.0",
    "react": "^18.3.1",
    "react-dom": "^18.3.1"
  },
  "devDependencies": {
    "@testing-library/jest-dom": "^6.6.0",
    "@testing-library/react": "^15.0.7",
    "@testing-library/user-event": "^14.6.0",
    "@types/react": "^18.3.20",
    "@types/react-dom": "^18.3.6",
    "@vitejs/plugin-react": "^4.5.0",
    "jsdom": "^26.1.0",
    "typescript": "^5.8.3",
    "vite": "^6.3.5",
    "vitest": "^3.2.4"
  }
}
```

- [ ] **Step 3: Create frontend HTML entry**

Create `frontend/index.html`:

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>All-in-One Testing</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

- [ ] **Step 4: Create TypeScript configuration**

Create `frontend/tsconfig.json`:

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "useDefineForClassFields": true,
    "lib": ["DOM", "DOM.Iterable", "ES2022"],
    "allowJs": false,
    "skipLibCheck": true,
    "esModuleInterop": true,
    "allowSyntheticDefaultImports": true,
    "strict": true,
    "forceConsistentCasingInFileNames": true,
    "module": "ESNext",
    "moduleResolution": "Node",
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "jsx": "react-jsx",
    "types": ["vitest/globals", "@testing-library/jest-dom"]
  },
  "include": ["src", "vitest.setup.ts"],
  "references": [{ "path": "./tsconfig.node.json" }]
}
```

Create `frontend/tsconfig.node.json`:

```json
{
  "compilerOptions": {
    "composite": true,
    "module": "ESNext",
    "moduleResolution": "Node",
    "allowSyntheticDefaultImports": true
  },
  "include": ["vite.config.ts"]
}
```

- [ ] **Step 5: Create Vite and Vitest configuration**

Create `frontend/vite.config.ts`:

```ts
import react from '@vitejs/plugin-react';
import { defineConfig } from 'vitest/config';

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://127.0.0.1:8080',
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: './vitest.setup.ts',
    globals: true,
  },
});
```

Create `frontend/vitest.setup.ts`:

```ts
import '@testing-library/jest-dom/vitest';
```

- [ ] **Step 6: Verify empty tooling**

Run:

```bash
cd backend
go test ./...
```

Expected: command exits with code 0 and either reports no packages or no test failures.

Run:

```bash
cd frontend
npm install
npm test -- --run --passWithNoTests
```

Expected: command exits with code 0.

- [ ] **Step 7: Commit tooling**

```bash
git add backend/go.mod frontend/package.json frontend/index.html frontend/tsconfig.json frontend/tsconfig.node.json frontend/vite.config.ts frontend/vitest.setup.ts
git commit -m "chore: add backend and frontend test tooling"
```

## Task 2: Backend Health Endpoint

**Files:**

- Test: `backend/internal/api/health_test.go`
- Create: `backend/internal/api/router.go`
- Create: `backend/cmd/server/main.go`

- [ ] **Step 1: Write the failing backend health test**

Create `backend/internal/api/health_test.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpointReturnsOK(t *testing.T) {
	router := NewRouter()
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("expected JSON response, got decode error: %v", err)
	}
	if body.Status != "ok" {
		t.Fatalf("expected status ok, got %q", body.Status)
	}
}
```

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```bash
cd backend
go test ./internal/api -run TestHealthEndpointReturnsOK -v
```

Expected: FAIL because `NewRouter` is undefined.

- [ ] **Step 3: Implement minimal backend router**

Create `backend/internal/api/router.go`:

```go
package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type healthResponse struct {
	Status string `json:"status"`
}

func NewRouter() http.Handler {
	router := chi.NewRouter()
	router.Get("/api/health", handleHealth)
	return router
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(healthResponse{Status: "ok"})
}
```

- [ ] **Step 4: Run the focused test and verify GREEN**

Run:

```bash
cd backend
go test ./internal/api -run TestHealthEndpointReturnsOK -v
```

Expected: PASS.

- [ ] **Step 5: Add minimal server entrypoint**

Create `backend/cmd/server/main.go`:

```go
package main

import (
	"errors"
	"log/slog"
	"net/http"
	"os"

	"all_in_one_testing/backend/internal/api"
)

func main() {
	addr := ":8080"
	if value := os.Getenv("APP_ADDR"); value != "" {
		addr = value
	}

	server := &http.Server{
		Addr:    addr,
		Handler: api.NewRouter(),
	}

	slog.Info("starting API server", "addr", addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("API server stopped", "error", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 6: Run backend package tests**

Run:

```bash
cd backend
go test ./...
```

Expected: PASS.

- [ ] **Step 7: Commit backend health endpoint**

```bash
git add backend/internal/api/health_test.go backend/internal/api/router.go backend/cmd/server/main.go backend/go.mod backend/go.sum
git commit -m "feat: add backend health endpoint"
```

## Task 3: Frontend Console Shell

**Files:**

- Test: `frontend/src/app/App.test.tsx`
- Create: `frontend/src/app/App.tsx`
- Create: `frontend/src/main.tsx`

- [ ] **Step 1: Write the failing frontend shell test**

Create `frontend/src/app/App.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react';
import App from './App';

describe('App', () => {
  it('shows the project workspace navigation', () => {
    render(<App />);

    expect(screen.getByRole('heading', { name: 'All-in-One Testing' })).toBeInTheDocument();
    expect(screen.getByRole('navigation', { name: 'Project workspace' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Dashboard' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Targets & Agents' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Scenarios' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Runs' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Reports' })).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```bash
cd frontend
npm test -- --run src/app/App.test.tsx
```

Expected: FAIL because `./App` cannot be resolved.

- [ ] **Step 3: Implement minimal frontend shell**

Create `frontend/src/app/App.tsx`:

```tsx
import { DashboardOutlined, FileTextOutlined, NodeIndexOutlined, PlayCircleOutlined, ProfileOutlined } from '@ant-design/icons';
import { Layout, Menu, Typography } from 'antd';

const navItems = [
  { key: 'dashboard', icon: <DashboardOutlined />, label: <a href="#dashboard">Dashboard</a> },
  { key: 'targets', icon: <NodeIndexOutlined />, label: <a href="#targets">Targets & Agents</a> },
  { key: 'scenarios', icon: <ProfileOutlined />, label: <a href="#scenarios">Scenarios</a> },
  { key: 'runs', icon: <PlayCircleOutlined />, label: <a href="#runs">Runs</a> },
  { key: 'reports', icon: <FileTextOutlined />, label: <a href="#reports">Reports</a> },
];

export default function App() {
  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Layout.Sider breakpoint="lg" collapsedWidth={0}>
        <nav aria-label="Project workspace">
          <Menu theme="dark" mode="inline" selectedKeys={['dashboard']} items={navItems} />
        </nav>
      </Layout.Sider>
      <Layout>
        <Layout.Header style={{ background: '#ffffff', borderBottom: '1px solid #e5e7eb' }}>
          <Typography.Title level={1} style={{ fontSize: 22, margin: 0 }}>
            All-in-One Testing
          </Typography.Title>
        </Layout.Header>
        <Layout.Content style={{ padding: 24 }}>
          <Typography.Title level={2} style={{ fontSize: 18 }}>
            Dashboard
          </Typography.Title>
        </Layout.Content>
      </Layout>
    </Layout>
  );
}
```

Create `frontend/src/main.tsx`:

```tsx
import React from 'react';
import ReactDOM from 'react-dom/client';
import App from './app/App';
import 'antd/dist/reset.css';

ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
```

- [ ] **Step 4: Run the focused test and verify GREEN**

Run:

```bash
cd frontend
npm test -- --run src/app/App.test.tsx
```

Expected: PASS.

- [ ] **Step 5: Run frontend build and tests**

Run:

```bash
cd frontend
npm run build
npm test -- --run
```

Expected: both commands exit with code 0.

- [ ] **Step 6: Commit frontend shell**

```bash
git add frontend/src/app/App.test.tsx frontend/src/app/App.tsx frontend/src/main.tsx frontend/package-lock.json
git commit -m "feat: add tested frontend console shell"
```

## Task 4: Frontend Health API Client

**Files:**

- Test: `frontend/src/api/health.test.ts`
- Create: `frontend/src/api/health.ts`

- [ ] **Step 1: Write the failing health API client test**

Create `frontend/src/api/health.test.ts`:

```ts
import { afterEach, describe, expect, it, vi } from 'vitest';
import { getHealth } from './health';

describe('getHealth', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('loads backend health status from the API', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ status: 'ok' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await getHealth();

    expect(fetchMock).toHaveBeenCalledWith('/api/health');
    expect(result).toEqual({ status: 'ok' });
  });

  it('throws a readable error when the backend returns a non-OK response', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      status: 503,
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(getHealth()).rejects.toThrow('Health check failed with HTTP 503');
  });
});
```

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```bash
cd frontend
npm test -- --run src/api/health.test.ts
```

Expected: FAIL because `./health` cannot be resolved.

- [ ] **Step 3: Implement minimal health API client**

Create `frontend/src/api/health.ts`:

```ts
export type HealthStatus = {
  status: 'ok';
};

export async function getHealth(): Promise<HealthStatus> {
  const response = await fetch('/api/health');
  if (!response.ok) {
    throw new Error(`Health check failed with HTTP ${response.status}`);
  }
  return response.json() as Promise<HealthStatus>;
}
```

- [ ] **Step 4: Run the focused test and verify GREEN**

Run:

```bash
cd frontend
npm test -- --run src/api/health.test.ts
```

Expected: PASS.

- [ ] **Step 5: Run all frontend tests**

Run:

```bash
cd frontend
npm test -- --run
```

Expected: PASS.

- [ ] **Step 6: Commit frontend health API client**

```bash
git add frontend/src/api/health.test.ts frontend/src/api/health.ts
git commit -m "feat: add tested frontend health API client"
```

## Task 5: Full Foundation Verification

**Files:**

- Verify: `backend/...`
- Verify: `frontend/...`

- [ ] **Step 1: Run all backend tests**

Run:

```bash
cd backend
go test ./...
```

Expected: PASS.

- [ ] **Step 2: Run all frontend tests**

Run:

```bash
cd frontend
npm test -- --run
```

Expected: PASS.

- [ ] **Step 3: Run frontend production build**

Run:

```bash
cd frontend
npm run build
```

Expected: PASS.

- [ ] **Step 4: Confirm git status**

Run:

```bash
git status --short
```

Expected: no uncommitted files.

## Plan Self-Review

Spec coverage for this foundation slice:

- Backend Go API foundation: covered by Task 1 and Task 2.
- Frontend React foundation: covered by Task 1 and Task 3.
- Frontend tests: covered by Task 3 and Task 4.
- TDD execution: enforced by the per-task RED/GREEN steps.
- Full verification: covered by Task 5.

The full product spec also requires Agent, Scenario, Executor, Adapter RPC, Profiling, Report, Auth, Project isolation, storage, and real-time monitoring. Those areas are outside this foundation slice and should be implemented through separate TDD plans after this plan is complete.
