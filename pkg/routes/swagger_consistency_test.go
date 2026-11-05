package routes

import (
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Este teste garante que a documentação Swagger acompanha as rotas:
//   - toda rota registrada em routes.go precisa estar em docs/swagger.json;
//   - toda rota do Swagger precisa existir (em routes.go ou num dos lugares
//     listados abaixo, onde as rotas são registradas fora deste arquivo).
//
// Se ele falhar depois de você mexer numa rota, rode `make swagger` e confira se
// a anotação @Router do handler está certa.

// Rotas registradas em routes.go de propósito sem documentação: infraestrutura.
var undocumentedByDesign = map[string]bool{
	"get /favicon.ico":  true,
	"get /manager":      true,
	"get /manager/*any": true,
	"get /swagger/*any": true,
}

// Prefixos documentados no Swagger cujas rotas são registradas fora de
// routes.go: /license em pkg/core (LicenseRoutes) e /passkey-ceremony em
// pkg/passkey.
var registeredElsewhere = []string{"/license/", "/passkey-ceremony/"}

var (
	groupRe = regexp.MustCompile(`\w+\s*:?=\s*eng\.Group\("([^"]*)"\)`)
	routeRe = regexp.MustCompile(`\b(eng|routes)\.(GET|POST|PUT|DELETE|PATCH)\("([^"]*)"`)
	paramRe = regexp.MustCompile(`:(\w+)`)
)

// registeredRoutes lê routes.go e devolve "metodo /caminho" no formato do Swagger
// (parâmetros como {id}).
func registeredRoutes(t *testing.T) map[string]bool {
	t.Helper()
	src, err := os.ReadFile("routes.go")
	if err != nil {
		t.Fatal(err)
	}

	routes := map[string]bool{}
	prefix := ""
	for _, line := range strings.Split(string(src), "\n") {
		if m := groupRe.FindStringSubmatch(line); m != nil {
			prefix = m[1]
			continue
		}
		if m := routeRe.FindStringSubmatch(line); m != nil {
			base := prefix
			if m[1] == "eng" {
				base = ""
			}
			path := paramRe.ReplaceAllString(base+m[3], "{$1}")
			routes[strings.ToLower(m[2])+" "+path] = true
		}
	}
	return routes
}

func documentedRoutes(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile("../../docs/swagger.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}

	docs := map[string]bool{}
	for path, methods := range spec.Paths {
		for method := range methods {
			docs[strings.ToLower(method)+" "+path] = true
		}
	}
	return docs
}

func sorted(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestRouteParserFindsTheRoutes(t *testing.T) {
	routes := registeredRoutes(t)
	// Se a expressão regular quebrar, o teste não pode passar em silêncio.
	if len(routes) < 80 {
		t.Fatalf("achei só %d rotas em routes.go; a leitura do arquivo quebrou", len(routes))
	}
	for _, want := range []string{
		"post /send/text",
		"post /send/pollVote",
		"get /instance/{instanceId}/advanced-settings",
		"put /instance/{instanceId}/advanced-settings",
	} {
		if !routes[want] {
			t.Errorf("rota esperada não encontrada: %q", want)
		}
	}
}

func TestEveryRegisteredRouteIsDocumentedInSwagger(t *testing.T) {
	docs := documentedRoutes(t)

	missing := map[string]bool{}
	for route := range registeredRoutes(t) {
		if !docs[route] && !undocumentedByDesign[route] {
			missing[route] = true
		}
	}
	if len(missing) > 0 {
		t.Fatalf("rotas registradas sem documentação no Swagger (anote o handler com @Router e rode `make swagger`):\n  %s",
			strings.Join(sorted(missing), "\n  "))
	}
}

func TestEverySwaggerRouteStillExists(t *testing.T) {
	routes := registeredRoutes(t)

	stale := map[string]bool{}
	for route := range documentedRoutes(t) {
		if routes[route] {
			continue
		}
		path := route[strings.Index(route, " ")+1:]
		elsewhere := false
		for _, prefix := range registeredElsewhere {
			if strings.HasPrefix(path, prefix) {
				elsewhere = true
			}
		}
		if !elsewhere {
			stale[route] = true
		}
	}
	if len(stale) > 0 {
		t.Fatalf("o Swagger documenta rotas que não existem mais (rode `make swagger`):\n  %s",
			strings.Join(sorted(stale), "\n  "))
	}
}
