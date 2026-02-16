# AGENTS.md

This file provides guidance and restrictions for coding agents like Claude Code, Github Copilot and Trae when working with this project.

## Project overview

An AI agent application with a Go-written backend service and a CLI app, still under construction, capable of tool calling, agentic looping and skills usage.

The backend service can also act as a MCP client to communicate with other MCP servers to extend its capabilities.

## Project structure

- `.agent/skills`: Contains skills that is vital for agents that working with this project.
- `cmd/`: Contains `main.go` for applications
- `pkg/`: Core codes and libraries used across the project
  - `config/`: Configuration management
  - `conn/`: Connections and clients
  - `customerrors/`: Custom error definitions
  - `middleware/`: Middlewares for GIN framework
  - `models/`: Data models and structures
  - `routes/`: API routes
  - `services/`: Business logic and services
  - `utils/`: Utility functions and helpers
  - `chat/`: LLM chat logic and tool calling functions (TODO)

## Agent guidelines

When coding agents for this project, please adhere to the following guidelines:

# [IMPORTANT] DO NOT WRITE ANY SUMMARY DOCUMENT UNLESS BEING TOLD TO DO SO.
# [IMPORTANT] YOU WILL BE HEAVILY PENALIZED IF YOU WRITE ANY SUMMARY DOCUMENT WITHOUT BEING TOLD TO DO SO.

1. **Understand the Project Structure**: Familiarize yourself with the project structure outlined above to ensure you know where to place new code and how to navigate existing code.

2. **Follow Coding Standards**: Follow Go coding standards and best practices. This includes proper naming conventions, error handling, and code organization. Do not write comment unless being told to do so.

3. **Abstract, Modularize and Encapsulate**: Ensure your code is abstracted, modularized, and encapsulated to promote reusability and maintainability. Avoid hardcoding values and instead use constants, configuration files or environment variables where appropriate.

4. **Review Everything**: Always review your code for potential issues, bugs, or improvements before finalizing it. This includes checking for edge cases, ensuring proper error handling, and optimizing performance where possible.

5. **Search Latest Info**: **CRITICAL** Stay updated with the latest information by searching Internet and using context7. This is crucial for ensuring that your code is relevant and effective in the current technological landscape. Always verify information from multiple sources to ensure accuracy.

## Other requirements

1. **Spoken Language**: Use Simplified Chinese when communicating with user.

2. **Use Skills**: Make use of the skills in `.agent/skills` when necessary. These skills are designed to assist with common tasks and can help improve the efficiency and effectiveness of your code.

3. **Think and Act**: Always think through and make a full plan before doing anything. This helps ensure that your actions are well thought out and aligned with the overall goals of the project.
