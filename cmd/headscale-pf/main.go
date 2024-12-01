package main

func main() {
	if err := cliCmd.Execute(); err != nil {
		errorInfo := map[string]any{
			"Step":  "Execute CLI",
			"Error": err.Error(),
		}
		logger.Fatal("Root error:", logger.ArgsFromMap(errorInfo))
	}
}
