package polish

import (
	"testing"
	"time"

	appcontext "github.com/AdeshDeshmukh/cadence/internal/context"
	"github.com/AdeshDeshmukh/cadence/internal/postprocess"
)

func TestRouter_DefaultCategoryRouting(t *testing.T) {
	router := NewRouter(nil)

	// Code
	if prof := router.Route("Visual Studio Code", appcontext.CategoryCode); prof != ProfileNameCode {
		t.Fatalf("expected code profile, got: %s", prof)
	}

	// Terminal
	if prof := router.Route("iTerm2", appcontext.CategoryTerminal); prof != ProfileNameCode {
		t.Fatalf("expected code profile for terminal, got: %s", prof)
	}

	// Medical
	if prof := router.Route("Epic Hyperspace", appcontext.CategoryMedical); prof != ProfileNameMedical {
		t.Fatalf("expected medical profile, got: %s", prof)
	}

	// Legal
	if prof := router.Route("Clio", appcontext.CategoryLegal); prof != ProfileNameLegal {
		t.Fatalf("expected legal profile, got: %s", prof)
	}

	// General
	if prof := router.Route("Safari", appcontext.CategoryBrowser); prof != ProfileNameGeneral {
		t.Fatalf("expected general profile for browser, got: %s", prof)
	}
}

func TestRouter_CustomRouteOverrides(t *testing.T) {
	initial := map[string]string{
		"slack": "slack_custom",
	}
	router := NewRouter(initial)

	// Check initial route
	if prof := router.Route("Slack", appcontext.CategoryChat); prof != "slack_custom" {
		t.Fatalf("expected slack_custom, got: %s", prof)
	}

	// Substring match
	if prof := router.Route("Acme Slack Desktop", appcontext.CategoryChat); prof != "slack_custom" {
		t.Fatalf("expected substring match to return slack_custom, got: %s", prof)
	}

	// Add new route
	router.SetRoute("Obsidian", "academic")
	if prof := router.Route("Obsidian", appcontext.CategoryDocument); prof != "academic" {
		t.Fatalf("expected academic for Obsidian, got: %s", prof)
	}

	// List routes
	routes := router.ListRoutes()
	if len(routes) != 2 || routes["slack"] != "slack_custom" || routes["obsidian"] != "academic" {
		t.Fatalf("unexpected routes list: %+v", routes)
	}

	// Delete route
	router.DeleteRoute("slack")
	if _, ok := router.GetRoute("slack"); ok {
		t.Fatalf("expected slack route to be deleted")
	}
	if prof := router.Route("Slack", appcontext.CategoryChat); prof != ProfileNameGeneral {
		t.Fatalf("expected general fallback after route deletion, got: %s", prof)
	}
}

func TestStep_AutoRouting(t *testing.T) {
	var requestedProfile string
	mock := NewMockPolisher(func(req Request) (Response, error) {
		requestedProfile = req.Profile
		return Response{Text: "polished"}, nil
	})

	router := NewRouter(map[string]string{
		"slack": "friendly_chat",
	})

	step := NewStep(mock, "general", 1*time.Second).WithRouter(router)

	// Case 1: Active application is VS Code -> should route to "code"
	step.Apply("def test()", postprocess.Context{
		Application: "Visual Studio Code",
	})
	if requestedProfile != "code" {
		t.Fatalf("expected auto-routed profile 'code', got %q", requestedProfile)
	}

	// Case 2: Active application is Slack -> should route to "friendly_chat"
	step.Apply("hey team", postprocess.Context{
		Application: "Slack",
	})
	if requestedProfile != "friendly_chat" {
		t.Fatalf("expected auto-routed profile 'friendly_chat', got %q", requestedProfile)
	}

	// Case 3: Step with explicit profile override (e.g. "medical")
	stepExplicit := NewStep(mock, "medical", 1*time.Second).WithRouter(router)
	stepExplicit.Apply("patient exam", postprocess.Context{
		Application: "Visual Studio Code",
	})
	if requestedProfile != "medical" {
		t.Fatalf("expected explicit profile 'medical' to override auto-routing, got %q", requestedProfile)
	}
}
