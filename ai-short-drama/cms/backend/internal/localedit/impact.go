package localedit

import "sort"

type Artifact struct {
	ID       string
	Type     string
	EntityID string
}

type Dependency struct {
	UpstreamID    string
	DownstreamID  string
	InvalidatesOn []string
}

type ArtifactImpact struct {
	Artifact Artifact
	Depth    int
	Path     []string
}

// CalculateImpact mirrors the database recursive walk. Only explicit dependency
// edges whose invalidates_on contains the plan's change kind can propagate.
func CalculateImpact(rootIDs []string, artifacts []Artifact, dependencies []Dependency, changeKind string, semantic bool) []ArtifactImpact {
	if !semantic || changeKind == "format_changed" || changeKind == "source_relocated" {
		return []ArtifactImpact{}
	}
	byID := make(map[string]Artifact, len(artifacts))
	edges := make(map[string][]Dependency)
	for _, artifact := range artifacts {
		byID[artifact.ID] = artifact
	}
	for _, dependency := range dependencies {
		edges[dependency.UpstreamID] = append(edges[dependency.UpstreamID], dependency)
	}
	type cursor struct {
		id    string
		depth int
		path  []string
	}
	queue := make([]cursor, 0, len(rootIDs))
	seen := make(map[string]int)
	for _, rootID := range rootIDs {
		queue = append(queue, cursor{id: rootID, path: []string{rootID}})
		seen[rootID] = 0
	}
	result := make([]ArtifactImpact, 0)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, dependency := range edges[current.id] {
			if !contains(dependency.InvalidatesOn, changeKind) || contains(current.path, dependency.DownstreamID) {
				continue
			}
			nextDepth := current.depth + 1
			if previousDepth, ok := seen[dependency.DownstreamID]; ok && previousDepth <= nextDepth {
				continue
			}
			artifact, ok := byID[dependency.DownstreamID]
			if !ok {
				continue
			}
			path := append(append([]string(nil), current.path...), dependency.DownstreamID)
			seen[dependency.DownstreamID] = nextDepth
			result = append(result, ArtifactImpact{Artifact: artifact, Depth: nextDepth, Path: path})
			queue = append(queue, cursor{id: dependency.DownstreamID, depth: nextDepth, path: path})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Depth == result[j].Depth {
			return result[i].Artifact.ID < result[j].Artifact.ID
		}
		return result[i].Depth < result[j].Depth
	})
	return result
}
