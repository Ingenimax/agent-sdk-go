### How to Use the Simple Agent Example
This example demonstrates how to create a basic AI agent using the Agent SDK with OpenAI as the LLM provider.

## Prerequisites
Before running the example, you'll need:

1. An OpenAI API key
2. (Optional) Google API key and Search Engine ID for web search functionality

## Setup
1. Set environment variables or update your configuration file:
```bash
# Required
export OPENAI_API_KEY=your_openai_api_key
# Optional (for web search functionality)
export GOOGLE_API_KEY=your_google_api_key
export GOOGLE_SEARCH_ENGINE_ID=your_google_search_engine_id
```
Create custom google eninge:
To get a Google Search API key and create a Google Search Engine, you'll need to first create a project in the Google Cloud Console, then enable the Custom Search API, and finally create an API key. You'll also need to create a Programmable Search Engine to get the Search Engine ID.
Here's a step-by-step guide:
1. Create a Google Cloud Project:
If you don't have one, create a Google Cloud project.
Go to the Google Cloud Console.
Select or create a project.
2. Enable the Custom Search API:
Navigate to the "APIs & Services" section.
Search for "Custom Search API" and enable it.
3. Create an API Key:
Go to the "Credentials" section within the APIs & Services.
Click "Create credentials" and select "API key".
Copy and securely store the API key.
4. Create a Programmable Search Engine:
Go to https://programmablesearchengine.google.com/controlpanel/create.
Create a new search engine.
Find the Search Engine ID in the Overview page's Basic section.


2. Build the example:
```go
go build -o simple_agent cmd/examples/simple_agent/main.go
```
## Running the Example
Run the compiled binary:
```bash
./simple_agent
```
The agent will execute with the default query "What's the weather like in San Francisco?" and display the response.

## Modifying the Example
To change the query or add more functionality:
1. Edit the main.go file to modify the query in the agent.Run() call
2. Add more tools to the createTools() function
3. Adjust the agent configuration with additional options

Example Output
When you run the example, you'll see output similar to:
```
I don't have real-time data access, but I can search the web for current weather in San Francisco.

[Searching the web for "current weather San Francisco"]

Based on my search, the current weather in San Francisco is partly cloudy with temperatures around 62°F (17°C). There's a light breeze from the west at about 10 mph. Humidity is approximately 75%. The forecast shows similar conditions throughout the day with temperatures ranging from 58°F to 65°F.

Would you like more specific information about the San Francisco weather forecast?
```

## Customization
You can customize the agent by:

* Changing the LLM model by modifying the WithModel() option
* Adding more tools to the registry
* Implementing a different memory system
* Adjusting the system prompt
