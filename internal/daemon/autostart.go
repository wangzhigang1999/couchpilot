package daemon

import (
	"bytes"
	"encoding/xml"
	"fmt"
)

const startupTaskName = "CouchPilot"

func startupTaskXML(username, executable, configPath string, verbose bool) (string, error) {
	quoteArgument := func(value string) string {
		return `"` + value + `"`
	}
	paths := RuntimePaths(configPath)
	arguments := fmt.Sprintf(`run --config %s --pid-file %s --stop-file %s`, quoteArgument(configPath), quoteArgument(paths.PIDFile), quoteArgument(paths.StopFile))
	if verbose {
		arguments += " --verbose"
	}
	escape := func(value string) (string, error) {
		var buffer bytes.Buffer
		if err := xml.EscapeText(&buffer, []byte(value)); err != nil {
			return "", err
		}
		return buffer.String(), nil
	}
	userXML, err := escape(username)
	if err != nil {
		return "", err
	}
	executableXML, err := escape(executable)
	if err != nil {
		return "", err
	}
	argumentsXML, err := escape(arguments)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Task version="1.4" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo><Description>Start CouchPilot at logon and restart it after failures.</Description></RegistrationInfo>
  <Triggers><LogonTrigger><Enabled>true</Enabled><UserId>%s</UserId></LogonTrigger></Triggers>
  <Principals><Principal id="Author"><UserId>%s</UserId><LogonType>InteractiveToken</LogonType><RunLevel>LeastPrivilege</RunLevel></Principal></Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <IdleSettings><StopOnIdleEnd>false</StopOnIdleEnd><RestartOnIdle>false</RestartOnIdle></IdleSettings>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <RunOnlyIfIdle>false</RunOnlyIfIdle>
    <WakeToRun>false</WakeToRun>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>7</Priority>
    <RestartOnFailure><Interval>PT1M</Interval><Count>10</Count></RestartOnFailure>
  </Settings>
  <Actions Context="Author"><Exec><Command>%s</Command><Arguments>%s</Arguments></Exec></Actions>
</Task>`, userXML, userXML, executableXML, argumentsXML), nil
}
