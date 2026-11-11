package game

import "testing"

// Execute bootstrap examples so deferred-ID and invalid view assertions cannot
// silently regress while the source merely continues to compile.
func TestBootstrapExamples(t *testing.T) {
	for name, run := range map[string]func(){
		"stats":         ExampleStatsPattern,
		"upgrade":       ExampleUpgradingEntityArchetype,
		"compare_stats": ExampleSharedStatsVsDenseStats,
		"shared":        ExampleSharedStats,
		"modify_shared": ExampleModifyingSharedStats,
		"compare":       ComparisonDenseVsShared,
	} {
		t.Run(name, func(t *testing.T) { run() })
	}
}
