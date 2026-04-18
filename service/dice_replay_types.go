package service

type DiceReplaySample struct {
	Value int
}

type DiceReplayEntry struct {
	RollIndex  int
	SourceText string
	Formula    string
	DetailText string
	ValueText  string
	ResultText string
	Samples    []DiceReplaySample
}

type DiceReplaySnapshot struct {
	Entries []DiceReplayEntry
}
