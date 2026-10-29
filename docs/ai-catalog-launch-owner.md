# Catalog launch owner（未發行）

這是隔離的 typed Catalog launch owner 實作，尚未代表 Engine／Web 整合、正式 Go CI 或 runtime 驗收通過。一般 Catalog API 保留原契約。

固定私有入口：

* `GET /v1-catalog/ai/launch-options?templateId=ctm-…&versionId=cvm-…`：只有安全 Question 定義，沒有 defaults、檔案、source digest 或答案。
* `GET /v1-catalog/ai/launch-material?templateId=ctm-…&versionId=cvm-…`：僅 Engine 使用，包含同一來源快照的 private files、原 Question、safe questionSchema、source/content digest 與既有 Catalog externalId。
* `POST /v1-catalog/ai/launch-validate`：封閉 body 為 `templateId/versionId/digest/safeAnswers/humanAnswers/referenceBindings`，回傳 private `digest/variables/missingHumanInputs`。`variables` 每個 question 都有明確字串值，對齊正常 Stack `map[string]` schema；false 與 0 分別成為 `"false"`、`"0"`。

入口只信真正 `RemoteAddr` 的 loopback peer 與單一、合法的 Engine project header。`X-Forwarded-For`、caller role、任意 URL 或額外 query/body 欄位不能建立權限。Engine 仍負責原 caller／Key 的 live RBAC、PLATFORM Catalog read、create/reference scope、revision／撤權與 explicit DENY；這些入口本身不簽發 grant。

每次讀取都由已授權 rows 重算 logical template/version identity；DB row ID 只作內部 locator。來源、版本、commit 或檔案變化會改 private digest；validate 會重讀並作 CAS。SQL 同時限制 project/global 與 catalog-template owner 一致，最多 64 files／1 MiB 原始內容，private JSON response 最多 4 MiB，validate request 最多 256 KiB。

password／masked／敏感欄位走 human channel；service／certificate／secret 走 Engine 已驗證的 reference channel。即使 optional 或來源有 default，human/reference 缺值仍未就緒；真人明確傳空字串可完成 optional 欄位。原始 default 不回填；缺值的 placeholder 不可送 materializer。安全 primitive、enum、長度、數值／字元條件與日期由同一 owner 驗證，錯誤只回固定 code。

私有 material／variables 不可進模型、公開 schema、一般 payload audit、WebStorage 或應用 log。Engine 需用既有 SecretsService 加密保存，並在每次解密前驗 actor/project/Key binding。正常 API 的既有 Question 解析已抽成共用 `parse.QuestionsFromFiles`，原先三種來源優先順序不變。
