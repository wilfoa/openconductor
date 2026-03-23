#!/bin/bash
# Sets up 4 diverse, unrelated demo repos for VHS recording.
set -e

rm -rf /tmp/oc-demo /tmp/oc-demo-config.yaml /tmp/oc-demo-state.json /tmp/oc-demo-logs
mkdir -p /tmp/oc-demo/{saas-app,home-infra/modules,local-poc,billing-api}

# ── saas-app: React/TypeScript SaaS frontend ─────────────────────
mkdir -p /tmp/oc-demo/saas-app/src
cat > /tmp/oc-demo/saas-app/package.json << 'EOF'
{
  "name": "saas-app",
  "version": "2.1.0",
  "dependencies": {
    "react": "^18.2.0",
    "react-dom": "^18.2.0",
    "typescript": "^5.0.0",
    "tailwindcss": "^3.4.0",
    "zustand": "^4.5.0"
  },
  "scripts": {
    "dev": "vite",
    "build": "tsc && vite build"
  }
}
EOF
cat > /tmp/oc-demo/saas-app/src/App.tsx << 'EOF'
import React from 'react';
import { useStore } from './store';

export default function App() {
  const { user, projects } = useStore();
  return (
    <div className="min-h-screen bg-gray-50">
      <header className="bg-white shadow-sm p-4">
        <h1 className="text-xl font-bold">Dashboard</h1>
      </header>
      <main className="p-6">
        <p>Welcome, {user?.name}</p>
        <p>{projects.length} active projects</p>
      </main>
    </div>
  );
}
EOF

# ── home-infra: Personal AWS infrastructure ──────────────────────
cat > /tmp/oc-demo/home-infra/README.md << 'EOF'
# Home Infrastructure
Personal AWS account infrastructure managed with Terraform.
Includes: VPC, ECS cluster, RDS, S3 buckets, CloudFront.
EOF

# ── local-poc: Quick Python proof-of-concept ─────────────────────
cat > /tmp/oc-demo/local-poc/main.py << 'EOF'
import argparse
import json

def analyze(data_path: str) -> dict:
    """Analyze JSON data and return summary stats."""
    with open(data_path) as f:
        data = json.load(f)
    return {
        "total_records": len(data),
        "fields": list(data[0].keys()) if data else [],
    }

def main():
    parser = argparse.ArgumentParser(description="Data analyzer POC")
    parser.add_argument("file", help="Path to JSON data file")
    args = parser.parse_args()
    result = analyze(args.file)
    print(json.dumps(result, indent=2))

if __name__ == "__main__":
    main()
EOF
cat > /tmp/oc-demo/local-poc/requirements.txt << 'EOF'
pandas>=2.0
matplotlib>=3.8
EOF

# ── billing-api: Go billing microservice ─────────────────────────
cat > /tmp/oc-demo/billing-api/main.go << 'EOF'
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type Invoice struct {
	ID     string  `json:"id"`
	Amount float64 `json:"amount"`
	Status string  `json:"status"`
}

func main() {
	http.HandleFunc("/invoices", func(w http.ResponseWriter, r *http.Request) {
		invoices := []Invoice{
			{ID: "inv_001", Amount: 99.99, Status: "paid"},
		}
		json.NewEncoder(w).Encode(invoices)
	})
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "ok")
	})
	http.ListenAndServe(":8080", nil)
}
EOF
cat > /tmp/oc-demo/billing-api/go.mod << 'EOF'
module github.com/mycompany/billing-api
go 1.24
EOF

# Init git repos
for d in saas-app home-infra local-poc billing-api; do
  (cd /tmp/oc-demo/$d && git init -q && git add -A && git commit -q -m 'init')
done

# Empty config
: > /tmp/oc-demo-config.yaml

echo "Demo repos ready: saas-app, home-infra, local-poc, billing-api"
