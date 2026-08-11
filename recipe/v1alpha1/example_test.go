// SPDX-License-Identifier: Apache-2.0
// Copyright © 2026 infraBuilder SASU and contributors

package v1alpha1_test

import (
	"errors"
	"fmt"

	v1alpha1 "github.com/tobby-fetch/recipe-spec/recipe/v1alpha1"
)

// ExampleParseRecipe parses a draft recipe and checks whether it is ready
// for publication (cooked profile).
func ExampleParseRecipe() {
	doc := []byte(`apiVersion: recipe.tobby.dev/v1alpha1
kind: Recipe
metadata:
  name: hello
  version: 1.0.0
spec:
  ingredients:
    - name: hello
      kind: ContainerImage
      ref: docker.io/library/hello-world
      version: "1.x"
`)
	r, err := v1alpha1.ParseRecipe(doc)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("%s %s: %d ingredient(s)\n", r.Metadata.Name, r.Metadata.Version, len(r.Spec.Ingredients))

	if err := r.Validate(v1alpha1.ProfileCooked); err != nil {
		var errs v1alpha1.ErrorList
		if errors.As(err, &errs) {
			fmt.Printf("not publishable, %d issue(s), first at %s\n", len(errs), errs[0].Path)
		}
	}
	// Output:
	// hello 1.0.0: 1 ingredient(s)
	// not publishable, 2 issue(s), first at spec.ingredients[0].digest
}

// ExampleParseConstraint resolves a version constraint against registry
// tags, per the rules of RECIPE-SPEC.md §9.2.
func ExampleParseConstraint() {
	c, err := v1alpha1.ParseConstraint(">=1.4.0 <2.0.0")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("exact:", c.IsExact())
	fmt.Println("matches 1.9.3:", c.Match("1.9.3"))

	tag, err := c.Resolve([]string{"1.0.0", "1.4.0", "1.9.3", "2.0.0-rc.1", "latest"})
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("resolved:", tag)
	// Output:
	// exact: false
	// matches 1.9.3: true
	// resolved: 1.9.3
}

// ExampleParse detects the document kind.
func ExampleParse() {
	doc := []byte(`apiVersion: recipe.tobby.dev/v1alpha1
kind: Retriever
metadata:
  name: restricted-zone
spec:
  cookbook: registry.example.com/cookbook
  recipes:
    - name: wordpress
      version: "6.8.2"
`)
	obj, err := v1alpha1.Parse(doc)
	if err != nil {
		fmt.Println(err)
		return
	}
	switch d := obj.(type) {
	case *v1alpha1.Recipe:
		fmt.Println("recipe", d.Metadata.Name)
	case *v1alpha1.Retriever:
		fmt.Println("retriever", d.Metadata.Name, "with", len(d.Spec.Recipes), "recipe(s)")
	}
	// Output:
	// retriever restricted-zone with 1 recipe(s)
}
