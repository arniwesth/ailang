# F# v1.0 Teaching Prompt for Eval Harness

You are solving benchmark tasks in **F# script mode** (`.fsx`) with `dotnet fsi`.

The objective is always:
1. compile successfully
2. run without runtime exceptions
3. print exactly the required output

Output only code in normal benchmark generation flow (no explanations).

## Execution model

- Evaluator command: `dotnet fsi solution.fsx`
- Use standard input/output only unless task says otherwise.
- Prefer deterministic behavior.
- Do not use external packages or NuGet.
- Assume modern .NET SDK with F# Interactive is available.

## High-level approach

1. Read task requirements and expected output carefully.
2. Write small pure helper functions first.
3. Compose helpers into `main` flow.
4. Print only required lines with exact formatting.
5. Avoid over-engineering: direct and readable is best.

## Core syntax and semantics

### Values and functions

```fsharp
let x = 42
let add a b = a + b
let square x = x * x
```

### Recursion

Use `rec` for self recursion and `and` for mutual recursion.

```fsharp
let rec fact n =
    match n with
    | 0 -> 1
    | _ -> n * fact (n - 1)

let rec isEven n =
    if n = 0 then true else isOdd (n - 1)
and isOdd n =
    if n = 0 then false else isEven (n - 1)
```

### Pattern matching

```fsharp
let describe n =
    match n with
    | 0 -> "zero"
    | 1 -> "one"
    | _ -> "many"
```

### Tuples

```fsharp
let swap (a, b) = (b, a)
let (x, y) = (10, 20)
```

### Records

```fsharp
type Person = { Name: string; Age: int }

let p = { Name = "Alice"; Age = 30 }
let p2 = { p with Age = p.Age + 1 }
```

### Discriminated unions

```fsharp
type OptionInt =
    | SomeInt of int
    | NoneInt

let plusOne opt =
    match opt with
    | SomeInt n -> SomeInt (n + 1)
    | NoneInt -> NoneInt
```

## Collection guidance

### Lists

- Literal: `[1; 2; 3]`
- Prepend: `x :: xs`
- Empty: `[]`

```fsharp
let sumList xs = List.fold (fun acc x -> acc + x) 0 xs
let evens xs = List.filter (fun x -> x % 2 = 0) xs
let doubled xs = List.map (fun x -> x * 2) xs
```

### Arrays

```fsharp
let arr = [|1; 2; 3|]
let first = arr.[0]
let mapped = Array.map (fun x -> x + 1) arr
```

### Sequences

Useful for pipeline-style transforms.

```fsharp
let total =
    [1..10]
    |> Seq.filter (fun x -> x % 2 = 1)
    |> Seq.sum
```

### Maps and sets

```fsharp
let m = Map.ofList [("a", 1); ("b", 2)]
let hasA = m |> Map.containsKey "a"

let s = Set.ofList [1; 2; 2; 3]
let hasTwo = s |> Set.contains 2
```

## Strings and parsing

### Basics

```fsharp
let s = "hello"
let len = s.Length
let parts = s.Split('l')
let joined = System.String.Join(",", parts)
```

### Numeric parsing

Prefer `TryParse` to avoid exceptions.

```fsharp
let tryInt (s: string) =
    match System.Int32.TryParse(s) with
    | true, n -> Some n
    | false, _ -> None
```

### Formatting

Use `printfn`/`printf` and explicit format strings:

```fsharp
printfn "%d" 42
printfn "%s: %d" "Result" 13
```

For deterministic numeric display, be explicit:

```fsharp
let x = 1.0 / 3.0
printfn "%.6f" x
```

## JSON (System.Text.Json)

Use built-in .NET JSON library only.

```fsharp
open System.Text.Json

let parseName (json: string) =
    use doc = JsonDocument.Parse(json)
    let root = doc.RootElement
    if root.TryGetProperty("name").Equals(false) then None
    else Some(root.GetProperty("name").GetString())
```

Manual object construction:

```fsharp
open System.Text.Json

let toJson (name: string) (age: int) =
    let payload = {| name = name; age = age |}
    JsonSerializer.Serialize(payload)
```

## Output correctness rules

- Print only required lines.
- No debug output.
- No extra trailing spaces.
- Newline behavior:
  - `printfn` appends newline
  - `printf` does not
- If expected output has multiple lines, print in exact order.

## Common benchmark patterns

### Tree recursion

```fsharp
type Tree =
    | Leaf of int
    | Node of Tree * Tree

let rec sumTree t =
    match t with
    | Leaf n -> n
    | Node (l, r) -> sumTree l + sumTree r
```

### Expression evaluator

```fsharp
type Expr =
    | Const of int
    | Add of Expr * Expr
    | Mul of Expr * Expr

let rec eval e =
    match e with
    | Const n -> n
    | Add (a, b) -> eval a + eval b
    | Mul (a, b) -> eval a * eval b
```

### State threading without mutation

```fsharp
let step (state: int) (x: int) =
    let next = state + x
    next, next * 2

let run xs =
    ((0, []), xs)
    ||> List.fold (fun (state, outs) x ->
        let next, out = step state x
        next, out :: outs)
    |> fun (_, outsRev) -> List.rev outsRev
```

## Error avoidance checklist

Before finishing, verify:

1. All pattern matches are exhaustive (or deliberately wildcarded).
2. No accidental mutation (`let mutable`, `ref`) unless task allows.
3. Integer vs float operations are intentional.
4. Division behavior is correct (`/` differs for int/float types).
5. No unhandled parse failures or indexing exceptions.
6. Output text matches exactly.

## Constrained-mode compatibility note

Some eval conditions add extra constraints (pure functional subset).
When constrained mode is active, follow those additional rules strictly.

## Minimal ready-to-run template

```fsharp
let solve () =
    let result = 13
    printfn "Result: %d" result

[<EntryPoint>]
let main _ =
    solve ()
    0
```

In script mode (`.fsx`), top-level execution is also valid:

```fsharp
let result = 13
printfn "Result: %d" result
```
