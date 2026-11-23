// The Sign tab's cryptographic half: a .p12 / .pfx certificate, its password, and
// a verb that signs the document and hands the signed file over.
//
// ⛔ SIGNING IS THE LAST STEP, NOT A STAGE OF THE PIPELINE. A signature is an
// incremental update that covers the FINAL bytes, so anything written after it
// — every verb in this workbench rewrites the document — would invalidate it.
// The signed bytes therefore go to the person and nowhere else: they are never
// opened, never kept in s.raw, never fed back to s.doc. Further edits produce an
// unsigned document, and the panel says so.
//
// ⛔ THE SECRETS. Neither the password nor the .p12 is logged, stored in the
// browser, put in a URL or quoted in a message: the messages below are fixed
// sentences chosen by the KIND of failure, never the text of an error. Both are
// let go after every attempt, whichever way it went (wipeSecrets).

package main

import (
	"errors"
	"strings"

	"github.com/go-pdfkit/ops"
	"github.com/go-widgets/toolkit"
)

// signPDF is ops.Sign, replaceable so a test can watch it fail.
var signPDF = ops.Sign

// signGroupCert is the part of the Sign panel that signs with a certificate.
func (s *state) signGroupCert(box *column) {
	box.add(toolkit.NewLabel("Digital signature — a .p12 or .pfx file"), bareH())
	box.add(s.secretRow("Certificate password", s.tools.p12Pw), labelledH())
	box.add(buttons(
		s.entryRow("Reason (optional)", "", s.tools.p12Reason),
		s.entryRow("Location (optional)", "", s.tools.p12Place),
	), labelledH())
	box.add(toolkit.NewLabel("Signing is the last step: the signed file is handed over, not opened here."), bareH())
	box.add(toolkit.NewLabel("Edit afterwards and you get an unsigned document."), bareH())
}

// signChoose asks for the certificate file.
//
// The host refuses a file over ops.MaxPKCS12Size BEFORE reading it, so a huge
// file never becomes a huge buffer in the page's memory; the length is checked
// again here because a host is only as careful as its author.
func (s *state) signChoose() {
	gen := s.p12Gen
	s.host.OpenCapped(ops.MaxPKCS12Size, func(name string, data []byte, tooLarge bool) {
		// ⛔ A wipe (another document, another tab, an attempt to sign) happened
		// while the file was being read: the certificate must not be held again.
		if gen != s.p12Gen {
			clear(data)
			return
		}
		s.wipeCert()
		if tooLarge || len(data) > ops.MaxPKCS12Size {
			clear(data)
			s.fail(p12Message(ops.ErrPKCS12TooLarge))
			return
		}
		s.p12, s.p12Name = data, name
		s.note = "certificate " + name + " chosen — type its password, then Sign and download"
		s.refresh()
	})
}

// signCert signs the document as it now stands and downloads the result.
func (s *state) signCert() {
	// ⛔ After the attempt, success or failure, the bytes and the password go.
	defer s.wipeSecrets()
	if s.doc == nil {
		s.fail("open a document first")
		return
	}
	if s.p12 == nil {
		s.fail("choose a .p12 or .pfx certificate first")
		return
	}
	var o ops.SignOptions
	if err := o.LoadPKCS12(s.p12, s.tools.p12Pw.Get()); err != nil {
		s.fail(p12Message(err))
		return
	}
	o.Reason = strings.TrimSpace(s.tools.p12Reason.Get())
	o.Location = strings.TrimSpace(s.tools.p12Place.Get())
	src, msg := s.saveBytes()
	if msg != "" {
		s.fail(msg)
		return
	}
	out, err := signPDF(src, o)
	if err != nil {
		s.fail("could not sign this document: " + err.Error())
		return
	}
	s.host.Save(signedName(s.name), out)
	s.note = "signed and handed over as " + signedName(s.name) + " — editing on gives an unsigned document"
	s.refresh()
}

// wipeCert lets go of the certificate file: its bytes are overwritten, then
// dropped.
func (s *state) wipeCert() {
	s.p12Gen++
	clear(s.p12)
	s.p12, s.p12Name = nil, ""
}

// wipeSecrets lets go of the certificate and empties the password box, which
// the binding carries to the widget.
func (s *state) wipeSecrets() {
	s.wipeCert()
	s.tools.p12Pw.Set("")
}

// signedName is what a signed document is offered under: the name with
// `_signed` before the extension, as BentoPDF's digital-signature tool names it.
func signedName(name string) string {
	if name == "" {
		return "document_signed.pdf"
	}
	if len(name) > 4 && strings.EqualFold(name[len(name)-4:], ".pdf") {
		return name[:len(name)-4] + "_signed.pdf"
	}
	return name + "_signed.pdf"
}

// p12Message says what went wrong with a certificate file, by the KIND of error
// ops gave. The error's own text is never quoted.
func p12Message(err error) string {
	switch {
	case errors.Is(err, ops.ErrPKCS12Password):
		return "the password is wrong, or cannot be used with this certificate"
	case errors.Is(err, ops.ErrPKCS12Unsupported):
		return "this certificate uses an algorithm or key type that cannot sign a PDF here"
	case errors.Is(err, ops.ErrPKCS12TooLarge):
		return "the certificate file is too large — a .p12 is a few kilobytes, the limit is 8 MiB"
	default:
		return "this is not a usable .p12 or .pfx file"
	}
}
