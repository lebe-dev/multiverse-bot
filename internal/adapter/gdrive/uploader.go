package gdrive

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"google.golang.org/api/drive/v3"
)

// uploadUserFile uploads filePath into the user's "Multiverse Bot" folder on their Drive.
// The folder is created automatically on first use and reused on subsequent uploads.
// Returns a view link.
func uploadUserFile(ctx context.Context, svc *drive.Service, title, filePath string) (string, error) {
	folderID, err := getOrCreateFolder(ctx, svc, "Multiverse Bot")
	if err != nil {
		return "", fmt.Errorf("preparing folder: %w", err)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("opening file: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Use video title as filename if available, keep original extension.
	name := filepath.Base(filePath)
	if title != "" {
		name = sanitizeFilename(title) + filepath.Ext(filePath)
	}

	file, err := svc.Files.Create(&drive.File{
		Name:    name,
		Parents: []string{folderID},
	}).Media(f).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("uploading to drive: %w", err)
	}

	return fmt.Sprintf("https://drive.google.com/file/d/%s/view", file.Id), nil
}

// getOrCreateFolder finds the "Multiverse Bot" folder or creates it.
// With drive.file scope, Files.List only sees folders the app itself created,
// so there is no risk of finding a user's unrelated folder with the same name.
func getOrCreateFolder(ctx context.Context, svc *drive.Service, name string) (string, error) {
	q := fmt.Sprintf("mimeType='application/vnd.google-apps.folder' and name='%s' and trashed=false", name)
	list, err := svc.Files.List().Q(q).Fields("files(id)").Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("listing folders: %w", err)
	}
	if len(list.Files) > 0 {
		return list.Files[0].Id, nil
	}

	folder, err := svc.Files.Create(&drive.File{
		Name:     name,
		MimeType: "application/vnd.google-apps.folder",
	}).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("creating folder: %w", err)
	}
	return folder.Id, nil
}

// sanitizeFilename removes characters that are invalid in Drive file names.
func sanitizeFilename(s string) string {
	var out []rune
	for _, r := range s {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			out = append(out, '_')
		default:
			out = append(out, r)
		}
	}
	if len(out) > 200 {
		out = out[:200]
	}
	return string(out)
}
