MSI packaging skeleton for xentz-agent.

Use WiX Toolset to create an MSI that:
- Installs to %ProgramFiles%\\XentzAgent\\
- Registers the Windows Service in system mode
- Supports silent install via msiexec with ENROLL_TOKEN/SERVER_URL/MODE
