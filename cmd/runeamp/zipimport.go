package main

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
	"unicode/utf8"
)

const (
	maxZIPUpload   int64 = 2 << 30
	maxZIPExpanded int64 = 8 << 30
	maxZIPEntries        = 50000
	maxZIPDepth          = 12
)

type zipReport struct {
	Path        string   `json:"path"`
	Tracks      int      `json:"tracks"`
	Archives    int      `json:"archives"`
	Duplicates  int      `json:"duplicates"`
	Skipped     int      `json:"skipped"`
	Bytes       int64    `json:"bytes"`
	BadArchives int      `json:"badArchives"`
	BadFiles    int      `json:"badFiles"`
	Warnings    []string `json:"warnings,omitempty"`
}
type zipImport struct {
	ctx     context.Context
	root    *os.Root
	work    *os.Root
	report  zipReport
	budget  int64
	entries int
}

func randomName() (string, error) {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", e
	}
	return ".zip-" + hex.EncodeToString(b[:]), nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(b []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	return r.r.Read(b)
}

// Only malformed archive data is recoverable. Limits, cancellation and disk
// failures must still abort the transaction.
var errDamagedZIP = errors.New("повреждённый или неподдерживаемый ZIP")

func damaged(e error) error { return fmt.Errorf("%w: %v", errDamagedZIP, e) }
func archiveReadError(e error) error {
	if e == nil || e == io.EOF {
		return e
	}
	var disk *os.PathError
	if errors.As(e, &disk) || errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
		return e
	}
	return damaged(e)
}

type zipDataReader struct{ r io.Reader }

func (r zipDataReader) Read(b []byte) (int, error) {
	n, e := r.r.Read(b)
	return n, archiveReadError(e)
}
func (z *zipImport) warn(name string, archive bool, e error) {
	if archive {
		z.report.BadArchives++
	} else {
		z.report.BadFiles++
	}
	if len(z.report.Warnings) < 10 {
		z.report.Warnings = append(z.report.Warnings, fmt.Sprintf("%s: %v", name, e))
	}
}

// Inspect the end record before the ZIP reader allocates the central directory.
func boundedZIP(f *os.File, size int64) (*zip.Reader, error) {
	if size > maxZIPUpload {
		return nil, errors.New("Превышен лимит размера ZIP")
	}
	if size < 22 {
		return nil, damaged(zip.ErrFormat)
	}
	n := int64(65557)
	if n > size {
		n = size
	}
	b := make([]byte, int(n))
	if _, e := f.ReadAt(b, size-n); e != nil {
		return nil, e
	}
	for i := len(b) - 22; i >= 0; i-- {
		if binary.LittleEndian.Uint32(b[i:]) != 0x06054b50 || i+22+int(binary.LittleEndian.Uint16(b[i+20:])) != len(b) {
			continue
		}
		if binary.LittleEndian.Uint16(b[i+4:]) != 0 || binary.LittleEndian.Uint16(b[i+6:]) != 0 {
			return nil, damaged(errors.New("Многотомные ZIP не поддерживаются"))
		}
		count := int(binary.LittleEndian.Uint16(b[i+10:]))
		central := binary.LittleEndian.Uint32(b[i+12:])
		if count == 65535 || central == 0xffffffff || binary.LittleEndian.Uint32(b[i+16:]) == 0xffffffff {
			return nil, damaged(errors.New("ZIP64 не поддерживается"))
		}
		if count > maxZIPEntries || central > 32<<20 {
			return nil, errors.New("Слишком много записей ZIP")
		}
		// Validate the real central directory, not just the claimed entry count.
		end := size - n + int64(i)
		offset := int64(binary.LittleEndian.Uint32(b[i+16:]))
		if offset+int64(central) != end || count != int(binary.LittleEndian.Uint16(b[i+8:])) {
			return nil, damaged(errors.New("Некорректный каталог ZIP (ZIP64/SFX не поддерживается)"))
		}
		var header [46]byte
		pos := offset
		for j := 0; j < count; j++ {
			if pos+46 > end {
				return nil, damaged(zip.ErrFormat)
			}
			if _, e := f.ReadAt(header[:], pos); e != nil {
				return nil, e
			}
			if binary.LittleEndian.Uint32(header[:]) != 0x02014b50 {
				return nil, damaged(zip.ErrFormat)
			}
			pos += 46 + int64(binary.LittleEndian.Uint16(header[28:])) + int64(binary.LittleEndian.Uint16(header[30:])) + int64(binary.LittleEndian.Uint16(header[32:]))
			if pos > end {
				return nil, damaged(zip.ErrFormat)
			}
		}
		if pos != end {
			return nil, damaged(zip.ErrFormat)
		}
		z, e := zip.NewReader(f, size)
		if e != nil && !errors.Is(e, zip.ErrInsecurePath) {
			if errors.Is(e, io.EOF) {
				return nil, damaged(e)
			}
			return nil, archiveReadError(e)
		}
		if z == nil || len(z.File) != count {
			return nil, damaged(zip.ErrFormat)
		}
		return z, nil
	}
	return nil, damaged(errors.New("Некорректный ZIP"))
}

// Preserve real directory paths. ZIPs nested in a directory expand into that directory,
// without a new directory named after each inner archive.
func zipPath(name string) (string, bool) {
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimSuffix(name, "/")
	if !utf8.ValidString(name) || name == "." || !validDirectory(name) || strings.Contains(name, ":") {
		return "", false
	}
	for _, r := range name {
		if r < 32 || r == 127 {
			return "", false
		}
	}
	return name, true
}
func (z *zipImport) copyEntry(f *zip.File, dst *os.File, limit int64) (int64, error) {
	if f.UncompressedSize64 > uint64(limit) || f.UncompressedSize64 > uint64(z.budget) {
		return 0, errors.New("Превышен лимит распаковки ZIP")
	}
	src, e := f.Open()
	if e != nil {
		return 0, archiveReadError(e)
	}
	defer src.Close()
	n, e := io.Copy(dst, io.LimitReader(contextReader{z.ctx, zipDataReader{src}}, int64(f.UncompressedSize64)+1))
	z.budget -= n
	if z.budget < 0 {
		return n, errors.New("Превышен лимит распаковки ZIP")
	}
	if e != nil {
		return n, e
	}
	if n != int64(f.UncompressedSize64) {
		return n, damaged(errors.New("Неверный размер файла ZIP"))
	}
	return n, nil
}
func (z *zipImport) extract(f *os.File, size int64, prefix string, depth int) error {
	if depth > maxZIPDepth {
		return errors.New("Слишком глубокая вложенность ZIP")
	}
	archive, e := boundedZIP(f, size)
	if e != nil {
		return e
	}
	z.report.Archives++
	for _, entry := range archive.File {
		if e = z.ctx.Err(); e != nil {
			return e
		}
		z.entries++
		if z.entries > maxZIPEntries {
			return errors.New("Слишком много записей во вложенных ZIP")
		}
		name := entry.Name
		if !utf8.ValidString(name) {
			const cp437 = "ÇüéâäàåçêëèïîìÄÅÉæÆôöòûùÿÖÜ¢£¥₧ƒáíóúñÑªº¿⌐¬½¼¡«»░▒▓│┤╡╢╖╕╣║╗╝╜╛┐└┴┬├─┼╞╟╚╔╩╦╠═╬╧╨╤╥╙╘╒╓╫╪┘┌█▄▌▐▀αßΓπΣσµτΦΘΩδ∞φε∩≡±≥≤⌠⌡÷≈°∙·√ⁿ²■ "
			chars := []rune(cp437)
			var decoded strings.Builder
			for _, b := range []byte(name) {
				if b < 128 {
					decoded.WriteByte(b)
				} else {
					decoded.WriteRune(chars[int(b)-128])
				}
			}
			name = decoded.String()
		}
		rel, ok := zipPath(name)
		if !ok {
			z.report.Skipped++
			continue
		}
		if entry.FileInfo().IsDir() {
			continue
		}
		if !entry.Mode().IsRegular() {
			z.report.Skipped++
			continue
		}
		target := path.Join(prefix, rel)
		if !validDirectory(target) {
			z.report.Skipped++
			continue
		}
		if strings.EqualFold(path.Ext(rel), ".zip") {
			temp, e := randomName()
			if e != nil {
				return e
			}
			nested, e := z.work.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
			if e != nil {
				return e
			}
			n, e := z.copyEntry(entry, nested, maxZIPUpload)
			if e == nil {
				e = z.extract(nested, n, path.Dir(target), depth+1)
			}
			ce := nested.Close()
			re := z.work.Remove(temp)
			if ce != nil {
				return ce
			}
			if re != nil {
				return re
			}
			if e != nil {
				if errors.Is(e, errDamagedZIP) {
					z.warn(target, true, e)
					continue
				}
				return fmt.Errorf("Вложенный архив %q: %w", rel, e)
			}
			continue
		}
		if entry.UncompressedSize64 > uint64(maxUpload) {
			z.report.Skipped++
			continue
		}
		if _, ok := makeTrack("music", target, int64(entry.UncompressedSize64)); !ok {
			z.report.Skipped++
			continue
		}
		if _, e = z.root.Lstat(target); e == nil {
			z.report.Duplicates++
			continue
		} else if !errors.Is(e, fs.ErrNotExist) && !errors.Is(e, unix.ENOTDIR) {
			return e
		}
		if e = z.root.MkdirAll(path.Dir(target), 0755); e != nil {
			if errors.Is(e, unix.ENOTDIR) || errors.Is(e, fs.ErrExist) {
				z.report.Duplicates++
				continue
			}
			return e
		}
		dst, e := z.root.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if errors.Is(e, fs.ErrExist) {
			z.report.Duplicates++
			continue
		}
		if e != nil {
			return e
		}
		n, e := z.copyEntry(entry, dst, maxUpload)
		ce := dst.Close()
		if ce != nil {
			return ce
		}
		if e != nil {
			if !errors.Is(e, errDamagedZIP) {
				return fmt.Errorf("Файл %q: %w", rel, e)
			}
			if re := z.root.Remove(target); re != nil {
				return re
			}
			// Remove empty parents created for this failed entry, but never a
			// directory containing previously recovered files.
			for dir := path.Dir(target); dir != "."; dir = path.Dir(dir) {
				if re := z.root.Remove(dir); re != nil {
					if errors.Is(re, unix.ENOTEMPTY) || errors.Is(re, unix.EEXIST) {
						break
					}
					return re
				}
			}
			z.warn(target, false, e)
			continue
		}
		z.report.Tracks++
		z.report.Bytes += n
	}
	return nil
}

func (a *App) importZIP(w http.ResponseWriter, r *http.Request) {
	select {
	case a.uploadSlots <- struct{}{}:
		defer func() { <-a.uploadSlots }()
	default:
		fail(w, 503, "Загрузка занята")
		return
	}
	source, allowed := a.uploadSource(r)
	if !allowed {
		fail(w, 403, "Хранилище только для чтения")
		return
	}
	dir := r.URL.Query().Get("dir")
	if dir == "" {
		dir = "."
	}
	root := a.roots[source]
	if !safeDirectory(root, dir) {
		fail(w, 400, "Папка назначения недоступна")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxZIPUpload+65536)
	mr, e := r.MultipartReader()
	if e != nil {
		fail(w, 400, "Ожидается multipart")
		return
	}
	part, e := mr.NextPart()
	if e != nil {
		fail(w, 400, "Нет архива")
		return
	}
	defer part.Close()
	name := part.FileName()
	base := strings.TrimSuffix(name, path.Ext(name))
	if !strings.EqualFold(path.Ext(name), ".zip") || !validDirectory(base) || base == "." || strings.ContainsAny(base, "/:") {
		fail(w, 400, "Некорректное имя ZIP")
		return
	}
	if _, ok := zipPath(base); !ok {
		fail(w, 400, "Некорректное имя ZIP")
		return
	}
	parent, e := root.OpenRoot(dir)
	if e != nil {
		fail(w, 400, "Папка недоступна")
		return
	}
	defer parent.Close()
	parentFD, e := parent.Open(".")
	if e != nil {
		fail(w, 500, "Папка недоступна")
		return
	}
	defer parentFD.Close()
	if _, e = parent.Lstat(base); e == nil {
		fail(w, 409, "Папка с именем архива уже существует")
		return
	} else if !errors.Is(e, fs.ErrNotExist) {
		fail(w, 500, "Не удалось проверить папку")
		return
	}
	temp, e := randomName()
	if e != nil {
		fail(w, 500, "Ошибка подготовки импорта")
		return
	}
	if e = parent.Mkdir(temp, 0700); e != nil {
		fail(w, 500, "Не удалось создать временную папку")
		return
	}
	defer parent.RemoveAll(temp)
	work, e := parent.OpenRoot(temp)
	if e != nil {
		fail(w, 500, "Ошибка подготовки импорта")
		return
	}
	defer work.Close()
	if e = work.Mkdir("result", 0755); e != nil {
		fail(w, 500, "Ошибка подготовки импорта")
		return
	}
	result, e := work.OpenRoot("result")
	if e != nil {
		fail(w, 500, "Ошибка подготовки импорта")
		return
	}
	defer result.Close()
	archive, e := work.OpenFile("source.zip", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if e != nil {
		fail(w, 500, "Ошибка подготовки импорта")
		return
	}
	defer archive.Close()
	n, e := io.Copy(archive, io.LimitReader(contextReader{r.Context(), part}, maxZIPUpload+1))
	if e != nil || n > maxZIPUpload {
		fail(w, 400, "ZIP превышает 2 ГиБ или загрузка прервана")
		return
	}
	z := zipImport{ctx: r.Context(), root: result, work: work, budget: maxZIPExpanded}
	if e = z.extract(archive, n, ".", 0); e != nil {
		fail(w, 422, "Импорт отменён: "+e.Error())
		return
	}
	if z.report.Tracks == 0 {
		fail(w, 422, "В архиве нет поддерживаемой музыки")
		return
	}
	full := path.Join(dir, base)
	if !validDirectory(full) {
		fail(w, 400, "Слишком длинный путь")
		return
	}
	// Prepare the entire index delta before publishing. No partially imported directory is visible.
	tracks := make([]Track, 0, z.report.Tracks)
	e = fs.WalkDir(result.FS(), ".", func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		if !validDirectory(path.Join(full, p)) {
			return errors.New("Слишком длинный путь")
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		if t, ok := makeTrack(source, path.Join(full, p), info.Size()); ok {
			tracks = append(tracks, classifyAY(t, result, p))
		}
		return nil
	})
	if e != nil {
		fail(w, 422, "Импорт отменён: "+e.Error())
		return
	}
	a.scanMu.Lock()
	defer a.scanMu.Unlock()
	if r.Context().Err() != nil {
		return
	}
	if !safeDirectory(root, dir) {
		fail(w, 409, "Папка назначения изменилась")
		return
	}
	current, e := root.Open(dir)
	if e != nil {
		fail(w, 409, "Папка назначения изменилась")
		return
	}
	oldInfo, e1 := parentFD.Stat()
	newInfo, e2 := current.Stat()
	current.Close()
	if e1 != nil || e2 != nil || !os.SameFile(oldInfo, newInfo) {
		fail(w, 409, "Папка назначения изменилась")
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	// Linux atomic, no-replace publication, including a concurrent external writer.
	fd := int(parentFD.Fd())
	if e = unix.Renameat2(fd, path.Join(temp, "result"), fd, base, unix.RENAME_NOREPLACE); e != nil {
		if errors.Is(e, fs.ErrExist) {
			fail(w, 409, "Папка с именем архива уже существует")
		} else {
			fail(w, 500, "Не удалось опубликовать импорт: "+e.Error())
		}
		return
	}
	for _, t := range tracks {
		t = a.resolveID(t)
		a.tracks[t.ID] = t
	}
	z.report.Path = full
	respond(w, z.report)
}
