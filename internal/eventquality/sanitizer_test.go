package eventquality

import "testing"

func TestSanitizeTitle_AcceptsPlainText(t *testing.T) {
	r := SanitizeTitle("MSCI rebalance announcement for Q3")
	if r.Rejected {
		t.Fatalf("plain text should not be rejected: %v", r.Reasons)
	}
	if r.Clean != "MSCI rebalance announcement for Q3" {
		t.Fatalf("clean=%q", r.Clean)
	}
}

func TestSanitizeTitle_RejectsHTMLTags(t *testing.T) {
	r := SanitizeTitle("Breaking <script>alert('xss')</script> news")
	if !r.Rejected {
		t.Fatal("HTML should be rejected")
	}
	if r.Clean != "Breaking alert('xss') news" {
		t.Fatalf("clean=%q", r.Clean)
	}
}

func TestSanitizeTitle_RejectsOver200Chars(t *testing.T) {
	long := ""
	for range 25 {
		long += "tenchar10"
	}
	long += "extra001"
	r := SanitizeTitle(long)
	if !r.Rejected {
		t.Fatal("long title should be rejected")
	}
}

func TestSanitizeTitle_RejectsAllDigits(t *testing.T) {
	r := SanitizeTitle("1234567890")
	if !r.Rejected {
		t.Fatal("all digits should be rejected")
	}
}

func TestSanitizeTitle_RejectsMixedHTMLAllDigits(t *testing.T) {
	r := SanitizeTitle("  <b>12345</b>67890  ")
	if !r.Rejected {
		t.Fatal("HTML+digits should be rejected")
	}
}

func TestSanitizeTitle_AcceptsAtBoundary(t *testing.T) {
	s := ""
	for range 200 {
		s += "x"
	}
	r := SanitizeTitle(s)
	if r.Rejected {
		t.Fatalf("200 chars should be accepted: %v", r.Reasons)
	}
}
