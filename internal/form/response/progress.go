package response

type SectionProgress string

const (
	SectionProgressSkipped    SectionProgress = "SKIPPED"
	SectionProgressNotStarted SectionProgress = "NOT_STARTED"
	SectionProgressDraft      SectionProgress = "DRAFT"
	SectionProgressCompleted  SectionProgress = "COMPLETED"
)
