package httpapi

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"imagetoolbox/internal/barcode"
	"imagetoolbox/internal/imageio"
)

func barcodeHandler(cfg Config, generate bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dir, err := os.MkdirTemp("", "itb-api-*")
		if err != nil {
			writeError(w, 500, err)
			return
		}
		defer func() { _ = os.RemoveAll(dir) }()
		f, err := parseMultipart(w, r, dir, cfg)
		if err != nil {
			writeError(w, multipartErrorStatus(err), err)
			return
		}
		if err = r.Context().Err(); err != nil {
			writeError(w, 504, err)
			return
		}
		if generate {
			opts, err := barcodeGenerateOptions(f)
			if err != nil {
				writeError(w, 400, err)
				return
			}
			plan, err := barcode.GeneratePlan(opts)
			if err != nil {
				writeError(w, operationErrorStatus(err), err)
				return
			}
			if err = validateImageSize(plan.Width, plan.Height, cfg); err != nil {
				writeError(w, operationErrorStatus(err), err)
				return
			}
			if plan.WorkingBytes > cfg.MaxWorkingBytes {
				writeError(w, 413, ErrImageTooLarge)
				return
			}
			path := filepath.Join(dir, "barcode.png")
			result, err := plan.WriteFile(r.Context(), path, false)
			if err != nil {
				writeError(w, operationErrorStatus(err), err)
				return
			}
			if err = r.Context().Err(); err != nil {
				writeError(w, 504, err)
				return
			}
			w.Header().Set("X-ITB-Barcode-Symbology", string(result.Symbology))
			serveFile(w, r, path, "barcode.png", 0, "barcode.generate")
			return
		}
		input, err := f.input("input", "symbology")
		if err != nil {
			writeError(w, 400, err)
			return
		}
		info, err := imageio.Probe(input.Path)
		if err != nil {
			writeError(w, operationErrorStatus(err), err)
			return
		}
		if err = validateImageSize(info.Width, info.Height, cfg); err != nil {
			writeError(w, operationErrorStatus(err), err)
			return
		}
		working, err := barcode.DecodeWorkingBytes(info.Width, info.Height)
		if err != nil || working > cfg.MaxWorkingBytes {
			writeError(w, 413, ErrImageTooLarge)
			return
		}
		var formats []barcode.Symbology
		if value, ok := f.values["symbology"]; ok {
			for _, s := range strings.Split(value, ",") {
				formats = append(formats, barcode.Symbology(s))
			}
		}
		result, err := barcode.DecodeFile(r.Context(), input.Path, barcode.DecodeOptions{Symbologies: formats})
		if err != nil {
			writeError(w, operationErrorStatus(err), err)
			return
		}
		if err = r.Context().Err(); err != nil {
			writeError(w, 504, err)
			return
		}
		result.Input.Path = input.OriginalName
		writeJSON(w, 200, result)
	}
}

func barcodeGenerateOptions(f form) (barcode.GenerateOptions, error) {
	if err := f.allowed("symbology", "data", "error-correction", "module-size", "border", "no-draw-text", "module-width", "module-height"); err != nil {
		return barcode.GenerateOptions{}, err
	}
	if len(f.files) != 0 {
		return barcode.GenerateOptions{}, fmt.Errorf("generate accepts scalar fields only")
	}
	opts := barcode.DefaultGenerateOptions()
	opts.Symbology = barcode.Symbology(f.values["symbology"])
	opts.Data = f.values["data"]
	if value, ok := f.values["error-correction"]; ok {
		opts.ErrorCorrection = value
	}
	for name, target := range map[string]*int{"module-size": &opts.ModuleSize, "border": &opts.Border, "module-width": &opts.ModuleWidth, "module-height": &opts.ModuleHeight} {
		if value, ok := f.values[name]; ok {
			n, err := strconv.Atoi(value)
			if err != nil {
				return barcode.GenerateOptions{}, fmt.Errorf("invalid %s: %w", name, err)
			}
			*target = n
		}
	}
	noText, err := f.bool("no-draw-text")
	if err != nil {
		return barcode.GenerateOptions{}, err
	}
	opts.DrawText = !noText
	return opts, nil
}
