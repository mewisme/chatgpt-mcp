package tuiguide

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

type Topic struct {
	ID          string
	Title       string
	Description string
	Keywords    []string
	File        string
	Parent      string
	Children    []string
}

//go:embed content
var files embed.FS

var topics = loadTopics()

func Topics() []Topic {
	result := make([]Topic, 0, len(topics))
	for _, topic := range topics {
		result = append(result, cloneTopic(topic))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func Children(parent string) []Topic {
	parent = cleanID(parent)
	result := []Topic{}
	for _, topic := range topics {
		if topic.Parent == parent {
			result = append(result, cloneTopic(topic))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Title < result[j].Title })
	return result
}

func Lookup(id string) (Topic, bool) {
	topic, ok := topics[cleanID(id)]
	return cloneTopic(topic), ok
}

func Markdown(id string) (string, error) {
	topic, ok := Lookup(id)
	if !ok {
		return "", fmt.Errorf("unknown TUI guide topic: %s", strings.TrimSpace(id))
	}
	data, err := files.ReadFile(topic.File)
	if err != nil {
		return "", fmt.Errorf("read TUI guide %s: %w", topic.ID, err)
	}
	return string(data), nil
}

func loadTopics() map[string]Topic {
	result := map[string]Topic{}
	_ = fs.WalkDir(files, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || path.Ext(name) != ".md" || name == "README.md" {
			return nil
		}
		id := topicIDFromFile(name)
		if id == "" {
			return nil
		}
		data, readErr := files.ReadFile(name)
		if readErr != nil {
			return nil
		}
		title, description := markdownMetadata(string(data), id)
		result[id] = Topic{ID: id, Title: title, Description: description, Keywords: topicKeywords(id, title), File: name, Parent: path.Dir(id)}
		if result[id].Parent == "." {
			result[id] = withParent(result[id], "")
		}
		return nil
	})
	for id, topic := range result {
		parent := topic.Parent
		if parent == "" {
			continue
		}
		if parentTopic, ok := result[parent]; ok {
			parentTopic.Children = append(parentTopic.Children, id)
			result[parent] = parentTopic
		}
	}
	for id, topic := range result {
		sort.Strings(topic.Children)
		result[id] = topic
	}
	return result
}

func topicIDFromFile(name string) string {
	name = strings.TrimPrefix(path.Clean(name), "./")
	name = strings.TrimPrefix(name, "content/")
	if path.Base(name) == "index.md" {
		return cleanID(path.Dir(name))
	}
	return cleanID(strings.TrimSuffix(name, ".md"))
}

func markdownMetadata(source, fallback string) (string, string) {
	lines := strings.Split(source, "\n")
	title := ""
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "# ") {
			title = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "# "))
			break
		}
	}
	if title == "" {
		title = titleFromID(path.Base(fallback))
	}
	description := ""
	paragraph := []string{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if len(paragraph) > 0 {
				description = strings.Join(paragraph, " ")
				break
			}
			continue
		}
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "```") || strings.HasPrefix(line, "|") || strings.HasPrefix(line, "-") {
			continue
		}
		paragraph = append(paragraph, line)
	}
	if description == "" {
		description = "Embedded TUI documentation for " + title + "."
	}
	return title, description
}

func topicKeywords(id, title string) []string {
	parts := strings.Fields(strings.NewReplacer("/", " ", "-", " ", "_", " ").Replace(id + " " + title))
	return append([]string{"guide", "help", "docs"}, parts...)
}

func titleFromID(id string) string {
	words := strings.Fields(strings.NewReplacer("-", " ", "_", " ").Replace(id))
	for i := range words {
		if words[i] != "" {
			words[i] = strings.ToUpper(words[i][:1]) + words[i][1:]
		}
	}
	return strings.Join(words, " ")
}

func cleanID(id string) string {
	id = strings.Trim(strings.ToLower(strings.TrimSpace(id)), "/")
	if id == "" {
		return ""
	}
	if id == "." {
		return ""
	}
	return path.Clean(id)
}

func cloneTopic(topic Topic) Topic {
	topic.Keywords = append([]string(nil), topic.Keywords...)
	topic.Children = append([]string(nil), topic.Children...)
	return topic
}

func withParent(topic Topic, parent string) Topic { topic.Parent = parent; return topic }
