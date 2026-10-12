package download

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type mediaInfo struct {
	Streams []struct {
		Index int    `json:"index"`
		Type  string `json:"codec_type"`
		Codec string `json:"codec_name"`
	} `json:"streams"`
	Format struct {
		Name string            `json:"format_name"`
		Tags map[string]string `json:"tags"`
	} `json:"format"`
}

func probe(ctx context.Context, run Runner, paths map[string]string, path string, log *jobLog, requireMP4 bool) (mediaInfo, error) {
	var info mediaInfo
	var data strings.Builder
	overflow := false
	stat, err := os.Stat(path)
	if err != nil || stat.Size() == 0 {
		return info, errors.New("Итоговый файл отсутствует или пуст")
	}
	code, err := run(ctx, paths["ffprobe.exe"], []string{"-v", "error", "-show_streams", "-show_format", "-of", "json", path}, filepath.Dir(path), func(stream, line string) {
		log.write("validation", "ffprobe", stream, line, nil)
		if stream == "stdout" {
			if data.Len()+len(line) > 1<<20 {
				overflow = true
			} else {
				data.WriteString(line)
			}
		}
	})
	log.exit("validation", paths["ffprobe.exe"], code, err)
	if err != nil || overflow || json.Unmarshal([]byte(data.String()), &info) != nil {
		return info, errors.New("Не удалось проверить медиа через ffprobe")
	}
	media := false
	for _, s := range info.Streams {
		if s.Type == "video" || s.Type == "audio" {
			media = true
		}
	}
	if !media {
		return info, errors.New("В файле нет медиа потоков")
	}
	if requireMP4 {
		brand := strings.TrimSpace(info.Format.Tags["major_brand"])
		if !strings.Contains(info.Format.Name, "mp4") || brand == "qt" || brand == "" {
			return info, errors.New("Файл не является MP4")
		}
	}
	return info, nil
}
func finalize(ctx context.Context, run Runner, paths map[string]string, input, temp string, log *jobLog) (string, error) {
	info, err := probe(ctx, run, paths, input, log, false)
	if err != nil {
		return "", err
	}
	output := filepath.Join(temp, "final.mp4")
	execute := func(out string, maps, codecs []string, trial bool) (bool, error) {
		args := []string{"-hide_banner", "-nostdin", "-y", "-i", input}
		args = append(args, maps...)
		args = append(args, codecs...)
		if trial {
			args = append(args, "-t", "0")
		}
		args = append(args, "-map_metadata", "-1", "-movflags", "+faststart", "-progress", "pipe:1", "-nostats", "-f", "mp4", out)
		unsupported := false
		diskFull := false
		code, e := run(ctx, paths["ffmpeg.exe"], args, temp, func(stream, line string) {
			log.write("finalizing", "ffmpeg", stream, line, nil)
			if stream == "stderr" && httpFailureMessage(line) == "Недостаточно места на диске" {
				diskFull = true
			}
			// An I/O/process failure is not evidence of codec incompatibility.
			// Only the MP4 muxer's explicit unsupported-codec diagnostic allows conversion.
			if stream == "stderr" && strings.Contains(line, "Could not find tag for codec") && strings.Contains(line, "codec not currently supported in container") {
				unsupported = true
			}
		})
		log.exit("finalizing", paths["ffmpeg.exe"], code, e)
		if e != nil && diskFull {
			e = errors.Join(e, windows.ERROR_DISK_FULL)
		}
		return unsupported, e
	}
	maps := []string{"-map", "0:v:0?", "-map", "0:a:0?"}
	if _, err = execute(output, maps, []string{"-c", "copy"}, false); err == nil {
		if _, err = probe(ctx, run, paths, output, log, true); err != nil {
			return "", err
		}
		log.write("finalizing", "ffmpeg", "policy", "MP4 stream-copy: video/audio сохранены", nil)
		return output, nil
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if filesystemFailure(err) != "" {
		return "", err
	}
	// Ask the installed muxer whether each selected stream is compatible. A zero
	// duration header trial distinguishes codec incompatibility from disk/input errors.
	codecs := []string{"-c", "copy"}
	converted := false
	seen := map[string]bool{}
	for _, s := range info.Streams {
		if (s.Type != "video" && s.Type != "audio") || seen[s.Type] {
			continue
		}
		seen[s.Type] = true
		trial := filepath.Join(temp, "trial-"+s.Type+".mp4")
		unsupported, e := execute(trial, []string{"-map", "0:" + strconv.Itoa(s.Index)}, []string{"-c", "copy"}, true)
		os.Remove(trial)
		if e != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			if !unsupported {
				return "", e
			}
			converted = true
			if s.Type == "video" {
				codecs = append(codecs, "-c:v", "libx264", "-crf", "18", "-preset", "medium")
			} else {
				codecs = append(codecs, "-c:a", "aac", "-b:a", "320k")
			}
			log.write("finalizing", "ffmpeg", "policy", fmt.Sprintf("Несовместимый %s codec %s: выборочное перекодирование", s.Type, s.Codec), nil)
		}
	}
	if !converted {
		return "", errors.New("Stream-copy не выполнен; кодеки совместимы, перекодирование не требуется")
	}
	if _, err = execute(output, maps, codecs, false); err != nil {
		return "", err
	}
	if _, err = probe(ctx, run, paths, output, log, true); err != nil {
		return "", err
	}
	return output, nil
}
