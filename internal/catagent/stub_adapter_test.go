package catagent_test

import (
	"testing"

	"FFmpegFree/internal/apperr"
	"FFmpegFree/internal/catagent"
)

func TestStubAdapterNotReady(t *testing.T) {
	a := catagent.NewStubAdapter(catagent.KindClaudeCLI)
	if a.Kind() != catagent.KindClaudeCLI {
		t.Fatal(a.Kind())
	}
	if a.Status().State != catagent.StateMissing {
		t.Fatalf("%+v", a.Status())
	}
	models, err := a.ListModels()
	if err != nil || len(models) != 0 {
		t.Fatalf("%v %v", models, err)
	}
	_, err = a.RunTurn(catagent.TurnOptions{})
	if !apperr.Is(err, apperr.CatNotReady) {
		t.Fatalf("%v", err)
	}
}

func TestPhase1CreatableThreeKinds(t *testing.T) {
	for _, k := range catagent.CreatableKinds() {
		if !catagent.Phase1Creatable(k) {
			t.Fatalf("%s", k)
		}
	}
	if catagent.Phase1Creatable("cat_cli") || catagent.Phase1Creatable("") {
		t.Fatal("unexpected")
	}
}
