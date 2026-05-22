// Package cert PDF generator — minimal pure-Go PDF 1.4 writer.
// No external dependencies; builds the object graph by hand.
package cert

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"strings"
	"time"
)

// GeneratePDF creates a PDF certificate document.
func GeneratePDF(cert *Certificate) []byte {
	var buf bytes.Buffer
	var objects []pdfObj
	var objNum int

	nextObj := func(data []byte) int {
		objNum++
		objects = append(objects, pdfObj{num: objNum, data: data})
		return objNum
	}

	// Font object
	fontData := []byte("<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>")
	fontObj := nextObj(fontData)

	// Page contents — flate-compressed for safety and size
	contents := buildContents(cert)
	var compressed bytes.Buffer
	w, _ := zlib.NewWriterLevel(&compressed, zlib.BestCompression)
	w.Write([]byte(contents))
	w.Close()
	contentLen := compressed.Len()

	// Content stream object with FlateDecode filter
	streamObj := nextObj([]byte(fmt.Sprintf("<< /Length %d /Filter /FlateDecode >>\nstream\n%s\nendstream", contentLen, compressed.String())))

	// Resources dict
	resources := fmt.Sprintf("<< /Font << /F1 %d 0 R >> >>", fontObj)

	// Page object
	pageData := fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents %d 0 R /Resources %s >>",
		streamObj, resources)
	pageObj := nextObj([]byte(pageData))

	// Pages object
	pagesData := fmt.Sprintf("<< /Type /Pages /Kids [%d 0 R] /Count 1 >>", pageObj)
	pagesObj := nextObj([]byte(pagesData))

	// Catalog
	catalogData := fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pagesObj)
	catalogObj := nextObj([]byte(catalogData))

	// Build PDF
	buf.WriteString("%PDF-1.4\n")

	// Write objects
	offsets := make([]int, len(objects))
	for i, obj := range objects {
		offsets[i] = buf.Len()
		buf.WriteString(fmt.Sprintf("%d 0 obj\n", obj.num))
		buf.Write(obj.data)
		buf.WriteString("\nendobj\n")
	}

	// Cross-reference table
	xrefOffset := buf.Len()
	buf.WriteString("xref\n")
	buf.WriteString(fmt.Sprintf("0 %d\n", len(objects)+1))
	buf.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		buf.WriteString(fmt.Sprintf("%010d 00000 n \n", off))
	}

	// Trailer
	buf.WriteString("trailer\n")
	buf.WriteString(fmt.Sprintf("<< /Size %d /Root %d 0 R >>\n", len(objects)+1, catalogObj))
	buf.WriteString("startxref\n")
	buf.WriteString(fmt.Sprintf("%d\n", xrefOffset))
	buf.WriteString("%%EOF\n")

	return buf.Bytes()
}

func buildContents(cert *Certificate) string {
	var b strings.Builder
	y := 740.0
	fontSize := 10.0

	bt := func(text string, size float64) {
		b.WriteString(fmt.Sprintf("BT /F1 %.0f Tf 50 %.0f Td (%s) Tj ET\n", size, y, pdfEscape(text)))
		y -= size + 4
	}

	section := func(title string) {
		y -= 4
		bt(title, 13)
		y -= 2
	}

	bt("CERTIFICATE OF ERASURE", 18)
	y -= 8
	bt(fmt.Sprintf("Generated: %s UTC", time.Now().UTC().Format(time.RFC3339)), 8)
	y -= 12

	section("Tool")
	bt(fmt.Sprintf("%s %s (built %s)", cert.Tool.Name, cert.Tool.Version, cert.Tool.BuildTime), fontSize)
	y -= 4

	section("Host")
	bt(fmt.Sprintf("Hostname: %s", cert.Host.Hostname), fontSize)
	bt(fmt.Sprintf("Kernel:   %s", cert.Host.Kernel), fontSize)
	y -= 4

	section("Device")
	bt(fmt.Sprintf("Path:   %s", cert.Device.SysfsPath), fontSize)
	bt(fmt.Sprintf("Model:  %s", cert.Device.Model), fontSize)
	bt(fmt.Sprintf("Serial: %s", cert.Device.Serial), fontSize)
	bt(fmt.Sprintf("Size:   %s bytes", cert.Device.Size), fontSize)
	y -= 4

	section("Wipe Operation")
	bt(fmt.Sprintf("Scheme:      %s (%s)", cert.Wipe.SchemeName, cert.Wipe.SchemeID), fontSize)
	bt(fmt.Sprintf("Passes:      %d", cert.Wipe.Passes), fontSize)
	bt(fmt.Sprintf("Started:     %s", cert.Wipe.StartedAt), fontSize)
	bt(fmt.Sprintf("Completed:   %s", cert.Wipe.CompletedAt), fontSize)
	bt(fmt.Sprintf("Duration:    %s", cert.Wipe.Duration), fontSize)
	bt(fmt.Sprintf("Data Written: %s bytes @ %s", cert.Wipe.BytesWritten, cert.Wipe.AvgSpeed), fontSize)
	y -= 4

	if cert.Wipe.PreHash != "" || cert.Wipe.PostHash != "" {
		section("Cryptographic Hashes")
		if cert.Wipe.PreHash != "" {
			bt(fmt.Sprintf("Pre-wipe SHA-256:  %s", cert.Wipe.PreHash), 8)
		}
		if cert.Wipe.PostHash != "" {
			bt(fmt.Sprintf("Post-wipe SHA-256: %s", cert.Wipe.PostHash), 8)
		}
		y -= 4
	}

	section("Verification")
	bt(fmt.Sprintf("Bytes Verified: %s", cert.Verification.BytesVerified), fontSize)
	bt(fmt.Sprintf("Result:         %s", cert.Verification.Result), fontSize)
	y -= 8

	section("Digital Signature")
	if cert.Signature != "" {
		bt(fmt.Sprintf("Ed25519 Signature: %s", cert.Signature), 7)
	} else {
		bt("(unsigned)", fontSize)
	}

	return b.String()
}

func pdfEscape(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "(", "\\(")
	s = strings.ReplaceAll(s, ")", "\\)")
	return s
}

type pdfObj struct {
	num  int
	data []byte
}
