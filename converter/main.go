package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func isValidURL(url string) bool {
	regex := `^(https?://)?(www\.)?(youtube\.com|youtu\.be)/.+$`
	matched, err := regexp.MatchString(regex, url)
	return matched && err == nil
}

func getVideoDuration(url string) (time.Duration, error) {
	cmd := exec.Command("yt-dlp", "--get-duration", url)
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	durationStr := strings.TrimSpace(string(output))
	return parseDuration(durationStr)
}

func parseDuration(duration string) (time.Duration, error) {
	parts := strings.Split(duration, ":")
	if len(parts) > 2 {
		return 0, errors.New("unexpected duration format")
	}

	seconds := 0
	for _, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil {
			return 0, errors.New("invalid number in duration")
		}
		seconds = seconds*60 + value
	}

	return time.Duration(seconds) * time.Second, nil
}

func getThumbnailURL(url string) (string, error) {
	cmd := exec.Command("yt-dlp", "--get-thumbnail", url)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func downloadThumbnail(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func cropThumbnail(imgData []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(imgData))
	if err != nil {
		return nil, err
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	size := width
	if height < width {
		size = height
	}
	x := (width - size) / 2
	y := (height - size) / 2

	croppedImg := image.NewRGBA(image.Rect(0, 0, size, size))
	for i := 0; i < size; i++ {
		for j := 0; j < size; j++ {
			croppedImg.Set(i, j, img.At(x+i, y+j))
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, croppedImg, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func downloadAudio(url string, thumbnailData []byte) error {
	if err := os.WriteFile("cover.jpg", thumbnailData, 0644); err != nil {
		return fmt.Errorf("failed to save thumbnail: %w", err)
	}
	defer os.Remove("cover.jpg")

	cmd := exec.Command(
		"yt-dlp",
		"-x",
		"--audio-format", "mp3",
		"--audio-quality", "0",
		"--output", "%(title)s.%(ext)s",
		"--no-keep-video",
		url,
	)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to download audio: %w", err)
	}

	files, err := os.ReadDir(".")
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	var mp3File string
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".mp3") {
			mp3File = file.Name()
			break
		}
	}

	if mp3File == "" {
		return errors.New("no MP3 file found after download")
	}

	cmd = exec.Command(
		"ffmpeg",
		"-i", mp3File,
		"-i", "cover.jpg",
		"-map", "0:0",
		"-map", "1:0",
		"-metadata", "title=ur the moon",
		"-metadata", "artist=Playboi Carti",
		"-c:a", "copy",
		"-c:v", "copy",
		"-id3v2_version", "3",
		"-disposition:v:0", "attached_pic",
		"output.mp3",
	)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add metadata: %w", err)
	}

	if err := os.Remove(mp3File); err != nil {
		return fmt.Errorf("failed to remove original mp3 file: %w", err)
	}

	return nil
}

func main() {
	url := "https://www.youtube.com/watch?v=sf0PJsknZiM"

	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		fmt.Printf("Conversion took %.2f seconds\n", elapsed.Seconds())
	}()

	if !isValidURL(url) {
		fmt.Printf("invalid YouTube link: %s\n", url)
		return
	}

	duration, err := getVideoDuration(url)
	if err != nil {
		fmt.Printf("error getting duration: %s\n", err)
		os.Exit(1)
	}
	if duration > (5 * time.Minute) {
		fmt.Printf("video is longer than 5 minutes: %d seconds\n", duration)
		os.Exit(1)
	}

	thumbnailURL, err := getThumbnailURL(url)
	if err != nil {
		fmt.Printf("error getting thumbnail URL: %s\n", err)
		os.Exit(1)
	}

	thumbnail, err := downloadThumbnail(thumbnailURL)
	if err != nil {
		fmt.Printf("error downloading thumbnail: %s\n", err)
		os.Exit(1)
	}

	croppedThumbnail, err := cropThumbnail(thumbnail)
	if err != nil {
		fmt.Printf("error cropping thumbnail: %s\n", err)
		os.Exit(1)
	}

	if err := downloadAudio(url, croppedThumbnail); err != nil {
		fmt.Printf("error downloading video: %s\n", err)
		os.Exit(1)
	}
}
