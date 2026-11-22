package follow

import "testing"

var paras = []string{
	"Hi, I'm Jose Acosta. I'm a Customer Success Manager based in Colombia.",
	"At Netskope I managed a book of more than 2,000 SaaS accounts and moved one from fifty to seventy percent adoption.",
	"Before that I spent two years at Fortinet as a support engineer, escalating the hardest enterprise cases.",
	"That is why I want this role.",
}

func TestFollowsSpeech(t *testing.T) {
	a := New(paras)
	p := a.Hear("hi I'm Jose Acosta")
	if !p.Accepted || p.Word != 4 || p.Para != 0 {
		t.Fatalf("start: %+v", p)
	}
	// whisper mistakes ("costumer", "Columbia") and a filler
	p = a.Hear("um I'm a costumer success manager based in Columbia")
	if p.Para != 1 || p.InPara != 0 {
		t.Fatalf("end of para 0: %+v", p)
	}
	// an overlapping window repeats words: no movement backwards
	p = a.Hear("success manager based in Colombia")
	if p.Para != 1 || p.InPara != 0 {
		t.Fatalf("overlap moved: %+v", p)
	}
	p = a.Hear("at Netskope I managed a book of more than 2000 SaaS")
	if p.Para != 1 || p.InPara != 11 {
		t.Fatalf("mid para 1: %+v", p)
	}
	// off script: nothing matches, position holds
	before := p.Word
	p = a.Hear("sorry let me get some water quickly")
	if p.Accepted || p.Word != before {
		t.Fatalf("off script: %+v", p)
	}
	// skipped a whole paragraph ahead
	p = a.Hear("escalating the hardest enterprise cases that is why")
	if p.Para != 3 || p.InPara != 3 {
		t.Fatalf("skip ahead: %+v", p)
	}
}

func TestSeek(t *testing.T) {
	a := New(paras)
	a.Seek(2)
	if p := a.Current(); p.Para != 2 || p.InPara != 0 {
		t.Fatalf("seek: %+v", p)
	}
	p := a.Hear("before that I spent two years")
	if p.Para != 2 || p.InPara != 6 {
		t.Fatalf("after seek: %+v", p)
	}
}

func TestSpanish(t *testing.T) {
	a := New([]string{"Hola, soy José. Trabajé en atención al cliente durante dos años."})
	p := a.Hear("hola soy jose trabaje en atencion al cliente")
	if p.InPara != 8 {
		t.Fatalf("spanish: %+v", p)
	}
}
