package domain

type Service struct {
	ID         int64
	CustomerID int64
	Name       string
	PrimaryCDN string
	BackupCDN  string
}
