package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"fm-live-radio/internal/generation"
	"fm-live-radio/internal/localtts/irodori/pipeline"
)

func main() {
	model := flag.String("model", "model/irodori-v4.1", "model directory")
	ep := flag.String("ep", "cpu", "cpu, cuda, or auto")
	seed := flag.Uint("seed", 0, "fixture seed")
	fixture := flag.String("fixture", "", "value-bearing fixture")
	fixtureCase := flag.String("fixture-case", "all", "fixture condition: all, null, reference, caption, reference+caption")
	durationScale := flag.Float64("duration-scale", 1, "duration predictor scale")
	smoke := flag.Bool("smoke", false, "run an additional Go/ORT synthesis smoke after numeric parity")
	text := flag.String("text", "こんにちは。", "smoke text")
	ref := flag.String("ref", "narrator/narrator_01.wav", "smoke reference WAV")
	outPath := flag.String("out", filepath.Join("model", "irodori-v4.1", "wp3-smoke.wav"), "smoke output WAV")
	steps := flag.Int("steps", 2, "smoke denoising steps")
	seconds := flag.Float64("seconds", 0.5, "smoke seconds")
	flag.Parse()
	secondsForParity := float64(0)
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "seconds" {
			secondsForParity = *seconds
		}
	})
	if *ep != "cpu" && *ep != "cuda" && *ep != "auto" {
		fatal(fmt.Errorf("unsupported execution provider %q", *ep))
	}
	if *durationScale <= 0 {
		fatal(fmt.Errorf("duration-scale must be positive"))
	}
	conditionCount, err := pipeline.FixtureConditionCount(*fixtureCase)
	if err != nil {
		fatal(err)
	}
	lib := generation.ResolveORTLibraryPathForEP(*ep)
	if lib == "" {
		fatal(fmt.Errorf("matching %s ORT DLL not found", *ep))
	}
	if err := generation.ConfigureExecutionProvider(*ep, 0); err != nil {
		fatal(err)
	}
	if err := generation.Init(lib); err != nil {
		fatal(err)
	}
	if *smoke {
		o := pipeline.DefaultOptions()
		o.ModelDir = *model
		o.Text = *text
		o.RefWAV = *ref
		o.OutputWAV = *outPath
		o.NumSteps = *steps
		o.Seconds = *seconds
		o.Seed = uint32(*seed)
		o.DurationScale = *durationScale
		rt, err := pipeline.LoadInitialise(o)
		if err != nil {
			fatal(fmt.Errorf("smoke load: %w", err))
		}
		if err := rt.Synthesize(); err != nil {
			fatal(fmt.Errorf("smoke synthesize: %w", err))
		}
		rt.Close()
		if _, err := os.Stat(*outPath); err != nil {
			fatal(fmt.Errorf("smoke output: %w", err))
		}
	}
	if err := pipeline.CompareV4FixtureWithOptions(*model, *fixture, pipeline.FixtureOptions{Seed: uint32(*seed), DurationScale: *durationScale, Seconds: secondsForParity, Case: *fixtureCase}); err != nil {
		fatal(err)
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"model": *model, "execution_provider": *ep, "seed": uint32(*seed), "fixture": *fixture, "fixture_case": *fixtureCase, "duration_scale": *durationScale, "seconds": secondsForParity, "numeric": true, "graphs": 6, "conditions": conditionCount, "steps": 40, "atol": 1e-4, "rtol": 1e-3, "harness": "product-v4-pipeline"})
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
