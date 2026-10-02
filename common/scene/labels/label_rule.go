package labels

import (
	"sort"
)

// LabelRule defines the rule for a given Label
type LabelRule struct {
	Label      string
	Threshold  float32
	Categories []string
	Priority   int
}

// LabelRules is a map of rules with label name as index
type LabelRules map[string]LabelRule

// LabelMaps are all predefined labels by label
var LabelMaps map[string]int

// LabelIDMaps are all predefined labels by ID
var LabelIDMaps map[int]string

func init() {
	lmap := map[string]struct{}{}
	for l, r := range rules {
		label := r.Label
		if label == "" {
			label = l
		}
		if label != "" {
			_, ok := lmap[label]
			if !ok {
				lmap[label] = struct{}{}
			}
		}
	}

	labels := []string{}
	for l := range lmap{
		labels = append(labels, l)
	}
	sort.Strings(labels)

	LabelMaps = map[string]int{}
	LabelIDMaps = map[int]string{}
	for id, l := range labels {
		LabelMaps[l] = id + 1
		LabelIDMaps[id + 1] = l
	}
}

// Find is a getter for LabelRules that give a default rule with a non-zero threshold for missing keys
func Find(label string) (rule LabelRule, ok bool) {
	if rule, ok := rules[label]; ok {
		return rule, true
	}

	return LabelRule{Threshold: 0.1}, false
}
