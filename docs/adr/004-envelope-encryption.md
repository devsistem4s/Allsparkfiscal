# ADR 004

Status: adotada na fundação de desenvolvimento.

Cada certificado/objeto recebe DEK aleatória cifrada com KEK, AES-256-GCM e AAD de contexto. KEK local é bootstrap; destino KMS/Vault com key_id, rotação e auditoria externa.
