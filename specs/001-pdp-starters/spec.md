# Feature Specification: PDP Conversation Starters (WebSocket)

**Feature Branch**: `001-pdp-starters`  
**Created**: 2026-03-08  
**Status**: Draft  
**Input**: User description: "Implementação do evento get_pdp_starters no weni-webchat-socket para fornecer conversation starters de forma assíncrona via invocação de Lambda AWS"

## Clarifications

### Session 2026-03-08

- Q: Should the system limit the number of concurrent Lambda invocations per instance to prevent resource exhaustion under high load? → A: Yes, bounded semaphore — cap concurrent invocations per pod via configurable env var; excess requests fail fast with error.
- Q: What should happen when the Lambda ARN env var is empty or not set? → A: Silent ignore — return nil without invoking Lambda or sending any response to the client; log at debug level only.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Requisição assíncrona de starters via WebSocket (Priority: P1)

Um cliente webchat conectado e registrado envia um evento `get_pdp_starters` pelo WebSocket com os dados do produto (account, linkText, productName, description, brand, attributes). O servidor recebe o evento, spawna uma goroutine separada para invocar a Lambda AWS (cujo ARN é definido via variável de ambiente), e retorna imediatamente o controle ao read loop do WebSocket. Quando a Lambda responde (podendo levar até 30 segundos), o servidor envia o resultado de volta ao cliente com um evento do tipo `starters` contendo as perguntas geradas.

**Why this priority**: É a funcionalidade central da feature — sem ela, nenhum conversation starter pode ser gerado e entregue ao cliente. Todo o valor da feature depende desta história.

**Independent Test**: Pode ser testado conectando um cliente WebSocket, registrando-o, enviando `get_pdp_starters` com dados de produto válidos e verificando que o evento `starters` é recebido com o array `questions`.

**Acceptance Scenarios**:

1. **Given** um cliente WebSocket registrado, **When** ele envia um evento `get_pdp_starters` com dados de produto válidos, **Then** o servidor spawna a invocação da Lambda em uma goroutine separada e o read loop não é bloqueado.
2. **Given** um evento `get_pdp_starters` em processamento, **When** a Lambda responde com sucesso contendo `questions`, **Then** o servidor envia ao cliente um `IncomingPayload` com `type: "starters"` e `data.questions` contendo as perguntas.
3. **Given** um evento `get_pdp_starters` em processamento, **When** a Lambda responde com sucesso, **Then** o cliente que enviou a requisição continua capaz de enviar e receber outras mensagens durante todo o tempo de espera.

---

### User Story 2 - Tratamento de erros na invocação da Lambda (Priority: P2)

Quando a invocação da Lambda falha (timeout, erro de invocação, resposta inválida), o servidor deve enviar ao cliente um payload de erro padrão do protocolo (`type: "error"`) e registrar logs estruturados do erro. A falha não deve afetar a conexão WebSocket nem o processamento de outros eventos do mesmo cliente ou de outros clientes.

**Why this priority**: Resiliência é essencial para que a feature não degrade a experiência do usuário nem a estabilidade do servidor. Erros na Lambda são esperados (cold start, throttling).

**Independent Test**: Pode ser testado configurando um ARN inválido ou simulando falha na Lambda e verificando que o cliente recebe o erro e continua operando normalmente.

**Acceptance Scenarios**:

1. **Given** um cliente registrado que enviou `get_pdp_starters`, **When** a invocação da Lambda falha (timeout, erro de rede, throttling), **Then** o servidor envia ao cliente um payload `type: "error"` com mensagem descritiva e não interrompe a conexão.
2. **Given** um cliente registrado que enviou `get_pdp_starters`, **When** a Lambda retorna uma resposta sem o campo `questions` ou com formato inválido, **Then** o servidor envia um payload `type: "error"` ao cliente.
3. **Given** múltiplos clientes conectados, **When** a invocação da Lambda de um cliente falha, **Then** os demais clientes não são afetados.

---

### User Story 3 - Validação de pré-condições (Priority: P3)

O servidor deve validar que o cliente está registrado (tem ID e Callback preenchidos) antes de processar o evento `get_pdp_starters`. Deve também validar que os dados mínimos do produto estão presentes no payload (ao menos `account` e `linkText`). Caso as validações falhem, o servidor retorna erro imediatamente sem invocar a Lambda.

**Why this priority**: Evita invocações desnecessárias da Lambda e segue o padrão de validação já existente nos outros handlers do websocket.

**Independent Test**: Pode ser testado enviando `get_pdp_starters` sem registro prévio ou com dados de produto incompletos e verificando o retorno de erro apropriado.

**Acceptance Scenarios**:

1. **Given** um cliente WebSocket que ainda não se registrou (sem ID/Callback), **When** ele envia `get_pdp_starters`, **Then** o servidor retorna erro `ErrorNeedRegistration`.
2. **Given** um cliente registrado, **When** ele envia `get_pdp_starters` sem os campos obrigatórios `account` ou `linkText` no `data`, **Then** o servidor retorna erro de validação sem invocar a Lambda.

---

### Edge Cases

- O que acontece quando o cliente se desconecta enquanto a goroutine da Lambda ainda está em execução? A goroutine deve detectar que o cliente não está mais acessível e fazer log sem panic.
- O que acontece quando o ARN da Lambda não está configurado na variável de ambiente? O handler ignora silenciosamente o evento (retorna `nil`), sem enviar resposta ao cliente, com log em nível debug apenas. Isso funciona como feature toggle implícito para ambientes onde a feature não está habilitada.
- O que acontece quando múltiplas requisições `get_pdp_starters` são enviadas pelo mesmo cliente em sequência rápida? Cada uma deve ser processada independentemente em sua própria goroutine.
- O que acontece quando a Lambda demora os 30 segundos completos de timeout? O read loop do WebSocket deve continuar funcional durante toda a espera.
- O que acontece quando o servidor está sob alta carga e muitos clientes enviam `get_pdp_starters` simultaneamente? Um semáforo limitado (configurável via env var) cap o número de invocações concorrentes por pod. Requisições que excedem o limite recebem erro imediato (fail fast) em vez de enfileirar ou spawnar goroutines ilimitadas.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: O sistema DEVE aceitar o evento `get_pdp_starters` no switch do `ParsePayload` em `client.go`, seguindo o padrão dos handlers existentes.
- **FR-002**: O sistema DEVE processar a invocação da Lambda em uma goroutine separada, garantindo que o read loop do WebSocket nunca seja bloqueado.
- **FR-003**: O sistema DEVE ler o ARN da Lambda a partir de uma variável de ambiente configurável.
- **FR-004**: O sistema DEVE invocar a Lambda AWS com o payload de entrada contendo os campos `account`, `linkText`, `productName`, `description`, `brand` e `attributes`, extraídos do campo `data` do `OutgoingPayload`. A invocação DEVE usar um timeout explícito configurável via variável de ambiente (padrão: 35 segundos) para evitar que a goroutine fique bloqueada indefinidamente caso o endpoint da Lambda esteja inacessível.
- **FR-005**: O sistema DEVE enviar ao cliente um `IncomingPayload` com `type: "starters"` e `data.questions` contendo o array de perguntas retornado pela Lambda.
- **FR-006**: O sistema DEVE validar que o cliente está registrado (ID e Callback preenchidos) antes de processar `get_pdp_starters`.
- **FR-007**: O sistema DEVE validar que os campos mínimos do produto (`account` e `linkText`) estão presentes no campo `data` do payload antes de invocar a Lambda.
- **FR-008**: O sistema DEVE enviar um payload `type: "error"` ao cliente quando a invocação da Lambda falha ou retorna resposta inválida.
- **FR-009**: O sistema DEVE registrar logs estruturados (com campos como `client_id`, `channel`, `lambda_arn`) para invocações com sucesso e falha.
- **FR-010**: O sistema DEVE tratar graciosamente o caso em que o cliente se desconecta durante o processamento da goroutine, sem panic ou goroutine leak.
- **FR-011**: O sistema DEVE limitar o número de invocações concorrentes da Lambda por instância usando um semáforo limitado, cujo valor máximo é configurável via variável de ambiente.
- **FR-012**: O sistema DEVE retornar um payload `type: "error"` imediatamente ao cliente quando o limite de concorrência for atingido, sem enfileirar a requisição.
- **FR-013**: Quando o ARN da Lambda não estiver configurado (env var vazia ou ausente), o sistema DEVE ignorar silenciosamente o evento `get_pdp_starters` — retornar sem erro, sem enviar resposta ao cliente, e registrar log apenas em nível debug.

### Key Entities

- **OutgoingPayload (existente)**: Payload enviado pelo cliente ao servidor. Já possui o campo `Data map[string]interface{}` que será usado para transportar os dados do produto.
- **IncomingPayload (existente)**: Payload enviado pelo servidor ao cliente. Já possui o campo `Data map[string]any` que será usado para transportar as perguntas no formato `{"questions": [...]}`.
- **Lambda Invocation**: Representação da chamada à função Lambda AWS. Recebe os dados do produto e retorna as perguntas geradas. O ARN é configurado via variável de ambiente.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Um cliente registrado que envia `get_pdp_starters` recebe as perguntas geradas dentro do tempo de resposta da Lambda (até 30 segundos), sem interrupção de outros eventos.
- **SC-002**: Durante o processamento de `get_pdp_starters` (incluindo os até 30 segundos de espera), o cliente consegue enviar e receber outros eventos normalmente (mensagens, ping, history).
- **SC-003**: Falhas na invocação da Lambda resultam em erro enviado ao cliente em 100% dos casos, sem interrupção da conexão WebSocket.
- **SC-004**: O processamento de `get_pdp_starters` em goroutine separada não causa degradação mensurável na latência de processamento dos demais eventos do WebSocket.
- **SC-005**: Nenhum goroutine leak ocorre mesmo em cenários de desconexão do cliente durante processamento.

## Assumptions

- A Lambda AWS já existe e é gerenciada por outra equipe/serviço. O socket apenas a invoca.
- O ARN da Lambda será fornecido via variável de ambiente, seguindo o padrão de configuração do projeto.
- O SDK da AWS está disponível ou será adicionado como dependência para invocar a Lambda.
- O cache (DynamoDB) é gerenciado internamente pela Lambda — o socket não tem conhecimento ou responsabilidade sobre cache.
- O timeout da Lambda é de até 30 segundos (configurado na própria Lambda), e o socket deve suportar esse tempo sem bloquear.
- O payload de resposta da Lambda segue o formato `{"questions": ["...", "...", "..."]}`.
- Apenas as 3 primeiras perguntas do array são relevantes (tratamento feito pelo frontend).
- O protocolo de eventos segue o padrão existente de `OutgoingPayload`/`IncomingPayload` já usado no projeto.
- Esta especificação cobre apenas o componente `weni-webchat-socket`. As implementações em `webchat-service` e `webchat-react` serão tratadas separadamente.

## Scope Boundaries

### In Scope

- Adição do case `"get_pdp_starters"` no `ParsePayload`
- Handler assíncrono com goroutine para invocação da Lambda
- Configuração do ARN da Lambda via variável de ambiente
- Validação de pré-condições (registro do cliente, dados mínimos do produto)
- Envio do resultado (`starters`) ou erro ao cliente
- Logging estruturado

### Out of Scope

- Implementação da Lambda AWS (cache, LLM, DynamoDB)
- Detecção de PDP no frontend
- Extração de dados do produto no frontend
- Exibição dos conversation starters na UI
- Comunicação entre `webchat-service` e `webchat-react`
- Rate limiting de requisições `get_pdp_starters` (pode ser adicionado futuramente)
- Métricas/observabilidade específicas para esta feature (pode ser adicionado futuramente)

