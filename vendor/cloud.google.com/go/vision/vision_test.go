// Copyright 2016 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package vision

import (
	"fmt"
	"log"
	"os"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/internal"
	"cloud.google.com/go/internal/testutil"
	gax "github.com/googleapis/gax-go"
	"golang.org/x/net/context"
	"google.golang.org/api/option"
)

type annotateTest struct {
	path string // path to image file, relative to testdata
	// If one of these is true, we expect that annotation to be non-nil.
	faces, landmarks, logos, labels, texts, fullText, web bool
	// We always expect safe search, image properties, and crop hints to be present.
}

func TestAnnotate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	tests := []annotateTest{
		{path: "face.jpg", faces: true, labels: true, web: true},
		{path: "cat.jpg", labels: true, web: true},
		{path: "faulkner.jpg", labels: true, web: true},
		{path: "mountain.jpg", texts: true, fullText: true, labels: true, web: true},
		{path: "no-text.jpg", labels: true, web: true},
		{path: "eiffel-tower.jpg", landmarks: true, labels: true, web: true},
		{path: "google.png", logos: true, labels: true, texts: true, fullText: true},
	}
	var wg sync.WaitGroup
	for _, test := range tests {
		test := test
		wg.Add(1)
		go func() {
			defer wg.Done()
			runAnnotateTest(t, ctx, client, test)
		}()
	}
	wg.Wait()
}

func runAnnotateTest(t *testing.T, ctx context.Context, client *Client, test annotateTest) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var anns *Annotations
	err := internal.Retry(ctx, gax.Backoff{
		Initial: 500 * time.Millisecond,
		Max:     2 * time.Second,
	}, func() (stop bool, err error) {
		annsSlice, err := client.Annotate(ctx, &AnnotateRequest{
			Image:        testImage(test.path),
			MaxFaces:     1,
			MaxLandmarks: 1,
			MaxLogos:     1,
			MaxLabels:    1,
			MaxTexts:     1,
			Web:          true,
			SafeSearch:   true,
			ImageProps:   true,
			CropHints:    &CropHintsParams{},
		})
		if err != nil {
			// Stop on RPC error.
			return true, err
		}
		anns = annsSlice[0]
		if anns.Error != nil {
			// Keep trying if the Error field is populated -- in practice
			// it is transient for these inputs.
			return false, fmt.Errorf("from Error field: %v", anns.Error)
		}
		return true, nil
	})
	if err != nil {
		t.Errorf("annotating %s: %v", test.path, err)
		return
	}
	p := map[bool]string{true: "present", false: "absent"}
	if got, want := (anns.Faces != nil), test.faces; got != want {
		t.Errorf("%s: faces %s, want %s", test.path, p[got], p[want])
	}
	if got, want := (anns.Landmarks != nil), test.landmarks; got != want {
		t.Errorf("%s: landmarks %s, want %s", test.path, p[got], p[want])
	}
	if got, want := (anns.Logos != nil), test.logos; got != want {
		t.Errorf("%s: logos %s, want %s", test.path, p[got], p[want])
	}
	if got, want := (anns.Labels != nil), test.labels; got != want {
		t.Errorf("%s: labels %s, want %s", test.path, p[got], p[want])
	}
	if got, want := (anns.Texts != nil), test.texts; got != want {
		t.Errorf("%s: texts %s, want %s", test.path, p[got], p[want])
	}
	if got, want := (anns.FullText != nil), test.fullText; got != want {
		t.Errorf("%s: full texts %s, want %s", test.path, p[got], p[want])
	}
	if got, want := (anns.SafeSearch != nil), true; got != want {
		t.Errorf("%s: safe search %s, want %s", test.path, p[got], p[want])
	}
	if got, want := (anns.ImageProps != nil), true; got != want {
		t.Errorf("%s: image properties %s, want %s", test.path, p[got], p[want])
	}
	if test.web {
		if got, want := (anns.Web != nil), true; got != want {
			t.Errorf("%s: web %s, want %s", test.path, p[got], p[want])
		}
	}
	if got, want := (anns.CropHints != nil), true; got != want {
		t.Errorf("%s: crop hints %s, want %s", test.path, p[got], p[want])
	}
}

func TestDetectMethods(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()
	var wg sync.WaitGroup
	for i, test := range []struct {
		path string
		call func(*Image) (bool, error)
	}{
		{"face.jpg",
			func(img *Image) (bool, error) {
				as, err := client.DetectFaces(ctx, img, 1)
				return as != nil, err
			},
		},
		{"eiffel-tower.jpg",
			func(img *Image) (bool, error) {
				as, err := client.DetectLandmarks(ctx, img, 1)
				return as != nil, err
			},
		},
		{"google.png",
			func(img *Image) (bool, error) {
				as, err := client.DetectLogos(ctx, img, 1)
				return as != nil, err
			},
		},
		{"faulkner.jpg",
			func(img *Image) (bool, error) {
				as, err := client.DetectLabels(ctx, img, 1)
				return as != nil, err
			},
		},
		{"mountain.jpg",
			func(img *Image) (bool, error) {
				as, err := client.DetectTexts(ctx, img, 1)
				return as != nil, err
			},
		},
		{"mountain.jpg",
			func(img *Image) (bool, error) {
				as, err := client.DetectDocumentText(ctx, img)
				return as != nil, err
			},
		},
		{"cat.jpg",
			func(img *Image) (bool, error) {
				as, err := client.DetectSafeSearch(ctx, img)
				return as != nil, err
			},
		},
		{"cat.jpg",
			func(img *Image) (bool, error) {
				ip, err := client.DetectImageProps(ctx, img)
				return ip != nil, err
			},
		},
		{"cat.jpg",
			func(img *Image) (bool, error) {
				as, err := client.DetectWeb(ctx, img)
				return as != nil, err
			},
		},
		{"cat.jpg",
			func(img *Image) (bool, error) {
				ch, err := client.CropHints(ctx, img, nil)
				return ch != nil, err
			},
		},
	} {
		i := i
		path := test.path
		call := test.call
		wg.Add(1)
		go func() {
			defer wg.Done()
			present, err := call(testImage(path))
			if err != nil {
				t.Errorf("%s, #%d: got err %v, want nil", path, i, err)
			} else if !present {
				t.Errorf("%s, #%d: nil annotation, want non-nil", path, i)
			}
		}()
	}
	wg.Wait()
}

// The DetectXXX methods of client that return EntityAnnotations.
var entityDetectionMethods = []func(*Client, context.Context, *Image, int) ([]*EntityAnnotation, error){
	(*Client).DetectLandmarks,
	(*Client).DetectLogos,
	(*Client).DetectLabels,
	(*Client).DetectTexts,
}

func TestErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	// Empty image.
	// With Client.Annotate, the RPC succeeds, but the Error field is non-nil.
	_, err := client.Annotate(ctx, &AnnotateRequest{
		Image:      &Image{},
		ImageProps: true,
	})
	if err != nil {
		t.Errorf("got %v, want nil", err)
	}

	// Invalid image.
	badImg := &Image{content: []byte("ceci n'est pas une image")}
	// If only ImageProps is specified, the result is an annotation
	// with all fields (including Error) nil. But any actual detection will fail.
	_, err = client.Annotate(ctx, &AnnotateRequest{
		Image:      badImg,
		SafeSearch: true,
	})
	if err != nil {
		t.Errorf("got %v, want error", err)
	}

	// With a Client.DetectXXX method, the Error field becomes the return value.
	_, err = client.DetectFaces(ctx, &Image{}, 1)
	if err == nil {
		t.Error("got nil, want error")
	}
	for i, edm := range entityDetectionMethods {
		_, err = edm(client, ctx, &Image{}, 1)
		if err == nil {
			t.Errorf("edm %d: got nil, want error", i)
		}
	}
	_, err = client.DetectSafeSearch(ctx, &Image{})
	if err == nil {
		t.Error("got nil, want error")
	}
	_, err = client.DetectImageProps(ctx, &Image{})
	if err == nil {
		t.Error("got nil, want error")
	}

	// Client.DetectXXX methods fail if passed a zero maxResults.
	img := testImage("cat.jpg")
	_, err = client.DetectFaces(ctx, img, 0)
	if err == nil {
		t.Error("got nil, want error")
	}
	for i, edm := range entityDetectionMethods {
		_, err = edm(client, ctx, img, 0)
		if err == nil {
			t.Errorf("edm %d: got nil, want error", i)
		}
	}
}

func integrationTestClient(ctx context.Context, t *testing.T) *Client {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	ts := testutil.TokenSource(ctx, Scope)
	if ts == nil {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}
	client, err := NewClient(ctx, option.WithTokenSource(ts))
	if err != nil {
		t.Fatal(err)
	}
	return client
}

var (
	mu     sync.Mutex
	images = map[string]*Image{}
)

func testImage(path string) *Image {
	mu.Lock()
	defer mu.Unlock()
	if img, ok := images[path]; ok {
		return img
	}
	f, err := os.Open("testdata/" + path)
	if err != nil {
		log.Fatal(err)
	}
	img, err := NewImageFromReader(f)
	if err != nil {
		log.Fatalf("reading image %q: %v", path, err)
	}
	images[path] = img
	return img
}
