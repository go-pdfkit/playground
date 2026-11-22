package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-pdfkit/ops"
	"github.com/go-pdfkit/reader"
	"github.com/go-pdfkit/reader/signature"
)

// The fixtures in testdata/p12 are ops' own (OpenSSL, PBES2/AES-256, HMAC-SHA-256
// MAC), protected by this password. They are test data and secret to nobody.
const p12Fixture = "fixture-only password"

// p12Bytes returns a FRESH copy each time: signing wipes the bytes it was given.
func p12Bytes(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "p12", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// certified is an opened document and a host that will hand back the named
// certificate.
func certified(t *testing.T, file string) (*state, *fakeHost) {
	t.Helper()
	s, h := opened(t, 2)
	h.p12, h.p12Name = p12Bytes(t, file), file
	s.signChoose()
	if s.p12 == nil {
		t.Fatalf("the certificate was not taken: %q", s.note)
	}
	return s, h
}

// TestSignRoundTrip signs with each fixture and verifies the result with the
// verifier ops' own tests use: reader/signature.
func TestSignRoundTrip(t *testing.T) {
	for _, file := range []string{"rsa-chain.p12", "ec.p12"} {
		t.Run(file, func(t *testing.T) {
			s, h := certified(t, file)
			s.tools.p12Pw.Set(p12Fixture)
			s.tools.p12Reason.Set("  I agree ")
			s.tools.p12Place.Set("Lyon")
			s.signCert()
			if h.as != "sample_signed.pdf" {
				t.Fatalf("handed over as %q (note %q)", h.as, s.note)
			}
			d, err := reader.Open(h.saved)
			if err != nil {
				t.Fatal(err)
			}
			rs, err := signature.VerifyAll(d)
			if err != nil || len(rs) != 1 {
				t.Fatalf("%d signatures, error %v", len(rs), err)
			}
			if rs[0].Err != nil || !rs[0].Signature.CoversWholeFile || !rs[0].Signature.DictionarySigned {
				t.Errorf("the signature does not verify: %+v", rs[0])
			}
			if !bytes.Contains(h.saved, []byte("I agree")) || !bytes.Contains(h.saved, []byte("Lyon")) {
				t.Error("the reason and the location are not in the signature")
			}
			if strings.Contains(s.note, p12Fixture) {
				t.Errorf("the note says the password: %q", s.note)
			}
		})
	}
}

// TestSigningLeavesThePipelineAlone: the signed bytes go to the person and
// nowhere else, so the document the workbench holds is the document it held.
func TestSigningLeavesThePipelineAlone(t *testing.T) {
	s, h := certified(t, "ec.p12")
	doc, raw, at, name := s.doc, append([]byte(nil), s.raw...), s.at, s.name
	before, err := docBytes(s.doc)
	if err != nil {
		t.Fatal(err)
	}
	s.tools.p12Pw.Set(p12Fixture)
	s.signCert()
	if h.saved == nil || !bytes.Contains(h.saved, []byte("/ByteRange")) {
		t.Fatalf("nothing signed was handed over: %q", s.note)
	}
	after, err := docBytes(s.doc)
	if err != nil {
		t.Fatal(err)
	}
	if s.doc != doc || !bytes.Equal(s.raw, raw) || s.at != at || s.name != name || !bytes.Equal(before, after) {
		t.Error("signing changed the document the workbench holds")
	}
	if bytes.Contains(s.raw, []byte("/ByteRange")) || bytes.Contains(after, []byte("/ByteRange")) {
		t.Error("the signed bytes were fed back into the pipeline")
	}
	if !strings.Contains(s.note, "unsigned") {
		t.Errorf("the note does not say further edits give an unsigned document: %q", s.note)
	}
	// An edit afterwards is an ordinary edit, and what it saves is unsigned.
	s.tools.spec.Set("1")
	pressVerb(t, s, "rotate")
	s.save()
	if bytes.Contains(h.saved, []byte("/ByteRange")) {
		t.Error("a later save carried a signature")
	}
}

// TestSigningLetsGoOfTheSecrets: after an attempt, whichever way it went, the
// bytes are overwritten and dropped and the password box is empty.
func TestSigningLetsGoOfTheSecrets(t *testing.T) {
	for _, tc := range []struct {
		name, password string
		ok             bool
	}{
		{"success", p12Fixture, true},
		{"wrong password", "not the password", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, h := certified(t, "ec.p12")
			held := s.p12
			s.tools.p12Pw.Set(tc.password)
			s.signCert()
			if (h.saved != nil) != tc.ok {
				t.Fatalf("signed %v, want %v: %q", h.saved != nil, tc.ok, s.note)
			}
			if s.p12 != nil || s.p12Name != "" || s.tools.p12Pw.Get() != "" {
				t.Error("the certificate or the password is still held")
			}
			if !bytes.Equal(held, make([]byte, len(held))) {
				t.Error("the certificate bytes were dropped but not overwritten")
			}
			for _, say := range []string{s.note, strings.Join(s.statusLine(), " ")} {
				if strings.Contains(say, tc.password) {
					t.Errorf("the workbench says the password: %q", say)
				}
			}
		})
	}
}

func TestSignRefusals(t *testing.T) {
	t.Run("no document", func(t *testing.T) {
		s := newState(surfaceW, surfaceH, &fakeHost{})
		s.signCert()
		if !strings.Contains(s.note, "open a document") {
			t.Errorf("note %q", s.note)
		}
	})
	t.Run("no certificate", func(t *testing.T) {
		s, h := opened(t, 1)
		s.tools.p12Pw.Set("x")
		s.signCert()
		if !strings.Contains(s.note, "choose a .p12") || h.saved != nil {
			t.Errorf("note %q", s.note)
		}
		if s.tools.p12Pw.Get() != "" {
			t.Error("the password was kept after a refused attempt")
		}
	})
	for _, tc := range []struct {
		name, file, password, want string
	}{
		{"wrong password", "ec.p12", "nope", "password is wrong"},
		{"empty password", "ec.p12", "", "password is wrong"},
		{"unsupported", "nomac.p12", p12Fixture, "algorithm or key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, h := certified(t, tc.file)
			s.tools.p12Pw.Set(tc.password)
			s.signCert()
			if !strings.Contains(s.note, tc.want) || h.saved != nil {
				t.Errorf("note %q, saved %v", s.note, h.saved != nil)
			}
		})
	}
	t.Run("not a p12", func(t *testing.T) {
		s, h := opened(t, 1)
		h.p12, h.p12Name = []byte("-----BEGIN CERTIFICATE-----"), "cert.pem"
		s.signChoose()
		s.tools.p12Pw.Set(p12Fixture)
		s.signCert()
		if !strings.Contains(s.note, "not a usable") || h.saved != nil {
			t.Errorf("note %q", s.note)
		}
	})
	t.Run("the document cannot be written", func(t *testing.T) {
		s, h := certified(t, "ec.p12")
		was := docBytes
		t.Cleanup(func() { docBytes = was })
		docBytes = func(*ops.Doc) ([]byte, error) { return nil, errors.New("the ink ran out") }
		s.tools.p12Pw.Set(p12Fixture)
		s.signCert()
		if !strings.Contains(s.note, "cannot be written") || h.saved != nil || s.p12 != nil {
			t.Errorf("note %q", s.note)
		}
	})
	t.Run("ops refuses to sign", func(t *testing.T) {
		s, h := certified(t, "ec.p12")
		was := signPDF
		t.Cleanup(func() { signPDF = was })
		signPDF = func([]byte, ops.SignOptions) ([]byte, error) { return nil, errors.New("no room") }
		s.tools.p12Pw.Set(p12Fixture)
		s.signCert()
		if !strings.Contains(s.note, "could not sign") || h.saved != nil || s.p12 != nil {
			t.Errorf("note %q", s.note)
		}
	})
}

// TestAnOversizedCertificateIsRefused: the host refuses it before reading, and
// the state refuses it again when a host hands one over regardless.
func TestAnOversizedCertificateIsRefused(t *testing.T) {
	big := func() []byte { return make([]byte, ops.MaxPKCS12Size+1) }
	t.Run("by the host", func(t *testing.T) {
		s, h := opened(t, 1)
		h.p12, h.p12Name = big(), "huge.p12"
		s.signChoose()
		if s.p12 != nil || !strings.Contains(s.note, "too large") {
			t.Errorf("held %v, note %q", s.p12 != nil, s.note)
		}
	})
	t.Run("by the state", func(t *testing.T) {
		data := big()
		s := newState(surfaceW, surfaceH, careless{&fakeHost{p12: data, p12Name: "huge.p12"}})
		s.signChoose()
		if s.p12 != nil || !strings.Contains(s.note, "too large") {
			t.Errorf("held %v, note %q", s.p12 != nil, s.note)
		}
		if data[0] != 0 || len(data) != ops.MaxPKCS12Size+1 {
			t.Error("unexpected data")
		}
	})
	t.Run("a second choice replaces the first", func(t *testing.T) {
		s, h := certified(t, "ec.p12")
		first := s.p12
		h.p12, h.p12Name = p12Bytes(t, "rsa-chain.p12"), "other.p12"
		s.signChoose()
		if s.p12Name != "other.p12" || !bytes.Equal(first, make([]byte, len(first))) {
			t.Error("the first certificate was not wiped")
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		s, h := opened(t, 1)
		s.signChoose()
		if s.p12 != nil || h.asked == 0 {
			t.Error("a cancelled picker held something")
		}
	})
}

// careless is a host that hands over whatever it holds, whatever the cap.
type careless struct{ *fakeHost }

func (c careless) OpenCapped(_ int, done func(string, []byte, bool)) {
	done(c.p12Name, c.p12, false)
}

func TestSignedName(t *testing.T) {
	for in, want := range map[string]string{
		"":          "document_signed.pdf",
		"a.pdf":     "a_signed.pdf",
		"A.PDF":     "A_signed.pdf",
		"report":    "report_signed.pdf",
		".pdf":      ".pdf_signed.pdf",
		"x.pdf.png": "x.pdf.png_signed.pdf",
	} {
		if got := signedName(in); got != want {
			t.Errorf("signedName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestTheSignPanelCarriesTheCertificateControls builds the panel and the verbs.
func TestTheSignPanelCarriesTheCertificateControls(t *testing.T) {
	s, h := opened(t, 1)
	h.p12, h.p12Name = p12Bytes(t, "ec.p12"), "ec.p12"
	pressVerb(t, s, "signChoose")
	if s.p12 == nil {
		t.Fatalf("the choose verb did not take the file: %q", s.note)
	}
	s.tools.p12Pw.Set(p12Fixture)
	pressVerb(t, s, "signCert")
	if h.as != "sample_signed.pdf" {
		t.Errorf("the sign verb handed over %q: %q", h.as, s.note)
	}
}

// p12Accept is what the certificate picker in web/index.html offers.
const p12Accept = ".p12,.pfx,application/x-pkcs12"

// TestTheCertificatePickerOffersOnlyACertificate: the page carries the input
// the browser host reads by id, and it offers a certificate and nothing else.
func TestTheCertificatePickerOffersOnlyACertificate(t *testing.T) {
	page, err := os.ReadFile(filepath.Join("web", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), `<input type="file" id="p12" accept="`+p12Accept+`">`) {
		t.Error("web/index.html has no certificate picker with id p12 offering exactly a .p12 or .pfx")
	}
}
