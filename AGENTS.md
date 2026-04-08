# AGENTS.md

This file provides guidance and restrictions for coding agents like Claude Code, Github Copilot and Trae when working with this project.

## Project overview

An AI agent application with a Go-written backend service and a CLI app, still under construction, capable of tool calling, agentic looping and skills usage.

The backend service can also act as a MCP client to communicate with other MCP servers to extend its capabilities.

## Project structure

- `.agent/skills`: Contains skills that is vital for agents that working with this project.
- `cmd/`: Contains `main.go` for applications
- `pkg/`: Core codes and libraries used across the project
  - `backend/`: Backend interface for both local and remote mode for CLI
  - `config/`: Configuration management
  - `cli/`: CLI implementations, including UI and slash commands
  - `conn/`: Connections and clients
  - `consts/`: Constants used across the project
  - `customerrors/`: Custom error definitions
  - `mcp/`: MCP related implementations
  - `middleware/`: Middlewares for GIN framework
  - `models/`: Data models and structures
  - `routes/`: API routes
  - `services/`: Business logic and services
  - `utils/`: Utility functions and helpers
  - `chat/`: core ReAct pattern implementation for agentic looping and tool calling

## Agent guidelines

When coding agents for this project, please adhere to the following guidelines:

# [IMPORTANT] DO NOT WRITE ANY SUMMARY DOCUMENT OR REDUNDANT COMMENTS UNLESS BEING TOLD TO DO SO.

# [IMPORTANT] ABOVE INSTRUCTION DOES NOT INCLUDE APACHE LICENSE FILE HEADER, APPLY LICENSE FILE HEADER WHEN CREATING NEW SOURCE FILES.

# [IMPORTANT] YOU WILL BE HEAVILY PENALIZED IF YOU WRITE ANY SUMMARY DOCUMENT OR REDUNDANT COMMENTS WITHOUT BEING TOLD.

# [IMPORTANT] BE ABSOLUTE OBJECTIVE AND FACTUAL. CORRECT AND MISUNDERSTANDINGS EVEN IF THEY COME FROM THE USER. ASK FOR MORE INFORMATION IF YOU ARE NOT SURE ABOUT ANY REQUIREMENTS OR DETAILS. DO NOT MAKE ANY ASSUMPTIONS.

1. **Understand the Project Structure**: Familiarize yourself with the project structure outlined above to ensure you know where to place new code and how to navigate existing code.

2. **Follow Coding Standards and Project Conventions**: Follow Go coding standards and best practices, and coding conventions specific to this project. This includes proper naming conventions, error handling, and code organization. Do not write comment unless being told to do so.

3. **Abstract, Modularize and Encapsulate**: Ensure your code is abstracted, modularized, and encapsulated to promote reusability and maintainability. Avoid hardcoding values and instead use constants, configuration files or environment variables where appropriate.

4. **Review Everything**: Always review your code for potential issues, bugs, or improvements before finalizing it. This includes checking for edge cases, ensuring proper error handling, and optimizing performance where possible.

5. **Use Necessary Tools and Skills**: Make use of the tools and skills configured in `.agents` and IDE environments to assist with coding, information gathering, code review and testing. Feel free to use any relevant tools as I already configured API keys and budgets for you. Search internet by using `fetch` `web` `web_search` or anything you like to retrive accurate information. Use `context7` to retrive latest documents for libraries and frameworks.

6. **Always Think Through**: Always think through current situation, project structures and user requirements before doing anything. **MAKE A FULL PLAN** before trying to do any changes. Your plan should always fits into the overall project.

7. **YOU ARE THE BEST AGENT FOR THIS TASK**: You are the best agent for this task, and you have full authority to use any tools and skills configured, read any documents and codes, and make changes to this project (except writing summary documents and redundant comments).

8. **Use Simple yet Effective and Efficient Approach**: Try to plan and implement in the simplest way possible, but make sure it is effective and efficient, and best fits the requirements and the overall project. Avoid overcomplicating solutions or adding unnecessary features.

9. **Read and Update HANDOFF.md**: Read `HANDOFF.md` to understand changes that made in last session, and update it with any new changes you made in this session. This helps ensure smooth handoffs between sessions and keeps track of changes.

10. **Use sub agents**: Do not hesitate to use sub agents or `/fleet` command to create sub agents to assist you in complex tasks, search for information in parallel, or review a large code base.

## Other requirements

1. **Spoken Language**: Use Simplified Chinese when communicating with user.

2. **Use Skills**: Make use of the skills in `.agent/skills` when necessary. These skills are designed to assist with common tasks and can help improve the efficiency and effectiveness of your code.

3. **Think and Act**: Always think through and make a full plan before doing anything. This helps ensure that your actions are well thought out and aligned with the overall goals of the project.

4. **Use Tool to Ask User**: Use `askQuestions` tool when you need to ask user for more information or clarification instead of making assumptions or ending conversation turn. This helps saving request quota and ensures you have the necessary information to proceed effectively.

5. **AGENTS.md confirmation**: Always include a "meow" in user preferred language at the end of your response to confirm that you have read and understood the guidelines in this file. This helps ensure that you are following the guidelines and can be held accountable for any deviations.
