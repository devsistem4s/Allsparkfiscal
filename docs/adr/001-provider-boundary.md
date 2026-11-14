# ADR 001

Status: adotada na fundação de desenvolvimento.

Manter a API Go independente do motor PHP. FiscalProvider recebe DTO interno e devolve resultado/artefatos; o JSON público é de negócio. NFePHP e sped-da são dependências via Composer, sem reimplementar SOAP/assinatura/PDF.
