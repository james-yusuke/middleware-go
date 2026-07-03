# Router Theory: Segment Lattice Router (SLR)

このリポジトリの router は、実装方式を Trie / Regex / Aho-Corasick のどれにするかよりも先に、**ルーティング意味論を固定する**べきです。SLR は「どのデータ構造で探索しても、同じ path は同じ handler chain に解決される」ことを目的にした理論です。

## 1. Path は文字列ではなく segment 列である

HTTP path を `/users/42/posts/7` のような文字列として扱うと、正規表現の登録順やワイルドカードの貪欲さに引っ張られます。SLR では path を次のような segment 列として正規化します。

```text
/users/42/posts/7 => ["users", "42", "posts", "7"]
/                 => []
```

route pattern も同じ segment 列に変換します。

```text
/users/:id/posts/:post_id => [Exact("users"), Param("id"), Exact("posts"), Param("post_id")]
/assets/*                 => [Exact("assets"), Wildcard("*")]
```

この変換により、router は「文字列マッチャ」ではなく「segment automaton」として扱えます。

## 2. 優先順位は lattice として定義する

同じ path に複数の route が一致する場合、登録順だけで決めると事故ります。SLR の一致優先度は次の lattice です。

```text
Exact segment > Param segment > Wildcard segment > NoMatch
```

route 全体の比較は以下の score で行います。

```text
score(route) = (
  staticSegmentCount desc,
  paramSegmentCount desc,
  wildcardCount asc,
  depth desc,
  registrationOrder asc
)
```

例: `/files/static` は `/files/:name` より強い。`/assets/logo.png` は `/assets/*` に落ちる。この規則を Router 実装全体で共有すれば、Trie でも Regex でも Aho-Corasick でも結果が揃います。

## 3. Handler chain は一度だけ合成する

middleware は route 探索の副産物ではなく、handler chain の合成規則です。

```text
FinalChain = GlobalMiddleware ⊕ GroupMiddleware ⊕ RouteHandlers
```

重要なのは、`GlobalMiddleware` を router にも engine にも二重登録しないことです。二重登録すると、ログ、認証、body limit、timeout などの副作用が 2 回発火します。今回の `_test.go` では、全 router mode で global middleware が 1 回だけ実行されることを契約にしています。

## 4. 三つの router 実装の位置づけ

### Trie Router

Trie は SLR の自然実装です。各 segment を node として進み、`Exact > Param > Wildcard` の順に探索します。計算量は path depth を `d` とすると概ね `O(d)` です。

### Regex Router

Regex は実装が短く、検証用・小規模用として便利です。ただし「最初に一致した route」ではなく、全候補から SLR score が最も高い route を選ぶ必要があります。これで `/things/:id` を先に登録しても `/things/new` が勝ちます。

### Aho-Corasick Router

Aho-Corasick は本来、複数 pattern の同時検索に強い automaton です。path router では文字単位ではなく segment 単位の automaton として使います。SLR では fail link を「候補回復」ではなく「segment 遷移の補助」として扱い、最終的な意味論は Trie と同じにします。

## 5. 革新的な点: Proof-carrying router

SLR の肝は、router の速さではなく **router が自分の正しさをテストで持ち運ぶ** ことです。

`router_contract_test.go` は以下を契約化します。

- root route が一致する
- method ごとに handler が分離される
- path param が抽出される
- static route が param route より優先される
- wildcard が残りの path を捕捉する
- middleware は handler chain に一度だけ入る

新しい router を追加する場合は、この契約テストの factory に 1 行足すだけです。データ構造を差し替えても意味論が崩れないため、性能改善と仕様安定を分離できます。

## 6. 次に入れると強い拡張

1. **Route migration**: `SwitchRouter` 時に既存 route table を新 router へ再登録する。
2. **Conflict detection**: `/users/:id` と `/users/:name` のような意味的重複を登録時に検出する。
3. **405 detection**: path は存在するが method が違う場合に `MethodNotAllowed` を返す。
4. **Route snapshot**: route table を export し、テスト・ドキュメント・OpenAPI 生成に使う。
5. **Adaptive router**: route 数や wildcard 率に応じて Trie / Regex / segment automaton を自動選択する。

SLR は「速い router」ではなく、**仕様が証明された router を高速化していくための足場**です。
