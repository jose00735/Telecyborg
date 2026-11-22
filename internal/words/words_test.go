package words

import (
	"reflect"
	"testing"
)

func TestFold(t *testing.T) {
	if got := Fold("¿Por qué TeleCyborg? ¡Educación, ação! I'm"); got != "por que telecyborg educacion acao im" {
		t.Fatalf("fold = %q", got)
	}
}

func TestStemAndClose(t *testing.T) {
	for _, c := range [][2]string{{"pricing", "prices"}, {"customers", "customer"}, {"integracion", "integraciones"}, {"netskope", "netscope"}} {
		if !Close(c[0], c[1]) {
			t.Errorf("%s ~ %s should match", c[0], c[1])
		}
	}
	for _, c := range [][2]string{{"cat", "car"}, {"team", "time"}, {"sale", "salary"}} {
		if Close(c[0], c[1]) {
			t.Errorf("%s ~ %s should not match", c[0], c[1])
		}
	}
}

func TestFillers(t *testing.T) {
	got := CountFillers("Um, so, like, I think, eh, o sea... este proyecto es bueno. Uhm you know, este, yes. Likely.")
	want := map[string]int{"um": 2, "like": 1, "eh": 1, "o sea": 1, "you know": 1, "este": 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fillers = %v", got)
	}
}
