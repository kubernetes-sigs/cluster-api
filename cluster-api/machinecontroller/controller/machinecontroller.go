package controller

func Run(c *Configuration) error {
	// Perform leader election.
	// When leader, do run(c)
	return run(c)
}

func run(c *Configuration) error {
	// To actual work
	return nil
}