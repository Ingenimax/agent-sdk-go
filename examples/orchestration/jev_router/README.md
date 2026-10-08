# Jev Router Example

This example uses TypeSafe AI's Jev System One model to choose an agent without
spending a generative LLM call on routing.

Jev is not registered as an `interfaces.LLM`: it does not generate text. The
`jev.Client` sends typed `noul`, `choice`, and `score` questions, while
`orchestration.JevRouter` adapts a Jev `choice` answer to the existing `Router`
interface.

```bash
export TYPESAFE_API_KEY=your_key
go run ./examples/orchestration/jev_router
```

Use `WithJevMinimumConfidence` to make uncertainty explicit. A decision below
the configured threshold returns `orchestration.ErrJevLowConfidence`, allowing
the application to ask for clarification or fall back to another router.
