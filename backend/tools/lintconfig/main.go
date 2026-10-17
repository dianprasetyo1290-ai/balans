// Command lintconfig menulis backend/.golangci.yml dari daftar modul dan tingkat dependensinya
// (Technical Guideline 2.3). Jalankan ulang setiap kali modul baru ditambahkan:
//
//	go run ./tools/lintconfig > .golangci.yml
package main

import (
	"fmt"
	"os"
	"slices"
	"strings"
)

const modulePath = "balans/internal/"

// depguard mencocokkan prefix; "app$" mencocokkan package wiring persis agar tidak ikut menolak
// modul berawalan sama seperti "approval".

// tiers mengikuti arah dependensi umum: modul hanya boleh mengimpor API publik modul di tingkat
// yang sama atau lebih rendah. Indeks lebih kecil = lebih rendah.
var tiers = [][]string{
	{"catalog", "party", "accounting", "tax", "document", "audit", "setting", "identity", "filestore", "notification", "dataexchange"},
	{"inventory", "warehouse", "logistics", "pricing", "approval", "hris"},
	{"storefront", "sales", "procurement", "vending", "reseller", "payroll", "pipeline", "crm"},
}

// alwaysOn adalah modul yang tidak diatur ENABLED_MODULES tetapi tetap mengikuti aturan batas modul.
var alwaysOn = []string{"system"}

type rule struct {
	name  string
	files []string
	deny  [][2]string
}

func allModules() []string {
	var all []string
	for _, t := range tiers {
		all = append(all, t...)
	}
	return append(all, alwaysOn...)
}

func tierOf(m string) int {
	for i, t := range tiers {
		if slices.Contains(t, m) {
			return i
		}
	}
	return len(tiers) // system berada di atas semua modul
}

func rules() []rule {
	modules := allModules()
	var out []rule

	platform := rule{name: "platform", files: []string{"**/internal/platform/**"}}
	for _, m := range modules {
		platform.deny = append(platform.deny, [2]string{modulePath + m, "platform tidak boleh bergantung pada modul"})
	}
	platform.deny = append(platform.deny,
		[2]string{modulePath + "integration", "platform tidak boleh bergantung pada adapter"},
		[2]string{modulePath + "app$", "platform tidak boleh bergantung pada wiring"})
	out = append(out, platform)

	for _, m := range modules {
		r := rule{name: "module-" + m, files: []string{"**/internal/" + m + "/**"}}
		for _, other := range modules {
			if other == m {
				continue
			}
			r.deny = append(r.deny, [2]string{modulePath + other + "/", "pakai API publik di root package " + other})
			if tierOf(other) > tierOf(m) {
				r.deny = append(r.deny, [2]string{modulePath + other, "arah dependensi salah: " + m + " tidak boleh bergantung pada " + other})
			}
		}
		r.deny = append(r.deny,
			[2]string{modulePath + "integration", "modul bergantung pada interface, bukan adapter"},
			[2]string{modulePath + "app$", "modul tidak boleh bergantung pada wiring"})
		out = append(out, r)
	}

	integration := rule{name: "integration", files: []string{"**/internal/integration/**"}}
	for _, m := range modules {
		integration.deny = append(integration.deny, [2]string{modulePath + m + "/", "adapter hanya memakai API publik modul " + m})
	}
	integration.deny = append(integration.deny, [2]string{modulePath + "app$", "adapter tidak boleh bergantung pada wiring"})
	out = append(out, integration)
	return out
}

func main() {
	var b strings.Builder
	b.WriteString(header)
	for _, r := range rules() {
		fmt.Fprintf(&b, "        %s:\n          list-mode: lax\n          files:\n", r.name)
		for _, f := range r.files {
			fmt.Fprintf(&b, "            - %q\n", f)
		}
		b.WriteString("          deny:\n")
		for _, d := range r.deny {
			fmt.Fprintf(&b, "            - pkg: %q\n              desc: %q\n", d[0], d[1])
		}
	}
	b.WriteString(footer)
	if _, err := os.Stdout.WriteString(b.String()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

const header = `# File ini dihasilkan oleh: go run ./tools/lintconfig > .golangci.yml
# Jangan diedit langsung; ubah tools/lintconfig/main.go.
version: "2"
run:
  tests: true
linters:
  default: none
  enable:
    - errcheck
    - govet
    - staticcheck
    - revive
    - gosec
    - depguard
    - gocritic
    - bodyclose
    - rowserrcheck
    - sqlclosecheck
    - contextcheck
    - nilerr
    - unused
    - ineffassign
  settings:
    revive:
      rules:
        - name: exported
          disabled: true
        - name: package-comments
          disabled: true
    gosec:
      excludes:
        - G104 # sudah ditangani errcheck
    depguard:
      rules:
`

const footer = `  exclusions:
    generated: lax
    rules:
      - path: _test\.go
        linters:
          - gosec
formatters:
  enable:
    - gofmt
    - goimports
  settings:
    goimports:
      local-prefixes:
        - balans
`
