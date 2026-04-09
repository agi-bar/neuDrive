package skillsarchive

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestParseZipBytes_MultiSkillArchive(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	write := func(name string, data []byte) {
		t.Helper()
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("Create(%s): %v", name, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("Write(%s): %v", name, err)
		}
	}
	write("demo-one/SKILL.md", []byte("# Demo One\n"))
	write("demo-one/helper.py", []byte("print('one')\n"))
	write("demo-two.skill/SKILL.md", []byte("# Demo Two\n"))
	write("demo-two.skill/assets/logo.png", []byte{0x89, 'P', 'N', 'G', 0x00})
	if err := zw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	entries, err := ParseZipBytes(buf.Bytes(), "skills.zip")
	if err != nil {
		t.Fatalf("ParseZipBytes: %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}
	if entries[0].SkillName != "demo-one" || entries[0].RelPath != "SKILL.md" {
		t.Fatalf("unexpected first entry: %+v", entries[0])
	}
	if entries[3].SkillName != "demo-two" || entries[3].RelPath != "assets/logo.png" {
		t.Fatalf("unexpected last entry: %+v", entries[3])
	}
}

func TestParseZipBytes_NamespacedMultiSkillArchive(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	write := func(name string, data []byte) {
		t.Helper()
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("Create(%s): %v", name, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("Write(%s): %v", name, err)
		}
	}
	write("public/docx/SKILL.md", []byte("# Docx\n"))
	write("public/docx/LICENSE.txt", []byte("license\n"))
	write("public/docx/scripts/run.py", []byte("print('docx')\n"))
	write("examples/brand-guidelines/SKILL.md", []byte("# Brand\n"))
	write("examples/brand-guidelines/assets/logo.txt", []byte("logo\n"))
	if err := zw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	entries, err := ParseZipBytes(buf.Bytes(), "claude-skills-all.zip")
	if err != nil {
		t.Fatalf("ParseZipBytes: %v", err)
	}
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}
	if entries[0].SkillName != "brand-guidelines" || entries[0].RelPath != "SKILL.md" {
		t.Fatalf("unexpected first entry: %+v", entries[0])
	}
	if entries[2].SkillName != "docx" || entries[2].RelPath != "LICENSE.txt" {
		t.Fatalf("unexpected docx license entry: %+v", entries[2])
	}
	if entries[3].SkillName != "docx" || entries[3].RelPath != "SKILL.md" {
		t.Fatalf("unexpected docx manifest entry: %+v", entries[3])
	}
	if entries[4].SkillName != "docx" || entries[4].RelPath != "scripts/run.py" {
		t.Fatalf("unexpected last entry: %+v", entries[4])
	}
}

func TestParseZipBytes_RequiresSkillManifest(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("demo/helper.py")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := w.Write([]byte("print('missing skill')\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := ParseZipBytes(buf.Bytes(), "demo.zip"); err == nil {
		t.Fatal("expected missing SKILL.md error")
	}
}

func TestParseZipBytes_RootSingleSkillArchive(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	write := func(name string, data []byte) {
		t.Helper()
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("Create(%s): %v", name, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("Write(%s): %v", name, err)
		}
	}
	write("SKILL.md", []byte("# Root Skill\n"))
	write("scripts/run.py", []byte("print('root')\n"))
	if err := zw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	entries, err := ParseZipBytes(buf.Bytes(), "root-demo.zip")
	if err != nil {
		t.Fatalf("ParseZipBytes: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].SkillName != "root-demo" || entries[0].RelPath != "SKILL.md" {
		t.Fatalf("unexpected first entry: %+v", entries[0])
	}
	if entries[1].SkillName != "root-demo" || entries[1].RelPath != "scripts/run.py" {
		t.Fatalf("unexpected second entry: %+v", entries[1])
	}
}

func TestInferArchiveSkillNameAndBinaryDetection(t *testing.T) {
	if got := InferArchiveSkillName("demo.skill.zip"); got != "demo" {
		t.Fatalf("unexpected inferred skill name: %q", got)
	}
	if !LooksBinary("assets/logo.png", []byte{0x89, 'P', 'N', 'G', 0x00}) {
		t.Fatal("expected png to be detected as binary")
	}
	if LooksBinary("SKILL.md", []byte("# Demo\n")) {
		t.Fatal("expected markdown to be detected as text")
	}
	if got := DetectContentType("helper.py", []byte("print('hi')\n")); got == "" {
		t.Fatal("expected content type")
	}
}
