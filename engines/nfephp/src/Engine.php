<?php
declare(strict_types=1);
namespace AllSpark\Fiscal;

use NFePHP\Common\Certificate;
use NFePHP\NFe\Make;
use NFePHP\NFe\Tools;
use NFePHP\NFe\Complements;
use NFePHP\DA\NFe\Danfe;
use NFePHP\DA\NFe\Danfce;
use NFePHP\DA\NFe\Dacce;

final class Engine
{
    private const UF = ['RO'=>11,'AC'=>12,'AM'=>13,'RR'=>14,'PA'=>15,'AP'=>16,'TO'=>17,'MA'=>21,'PI'=>22,'CE'=>23,'RN'=>24,'PB'=>25,'PE'=>26,'AL'=>27,'SE'=>28,'BA'=>29,'MG'=>31,'ES'=>32,'RJ'=>33,'SP'=>35,'PR'=>41,'SC'=>42,'RS'=>43,'MS'=>50,'MT'=>51,'GO'=>52,'DF'=>53];
    private const TAGS = [
        'ide','refNFe','refNF','refNFP','refCTe','refECF',
        'emit','enderEmit','dest','enderDest','retirada','entrega','autXML',
        'prod','infAdProd','CEST','Rastro','med','comb','encerrante','origComb',
        'imposto','ICMS','ICMSPart','ICMSST','ICMSSN','ICMSUFDest',
        'IPI','II','PIS','PISST','COFINS','COFINSST','ISSQN','impostoDevol',
        'IS','ISTot','IBSCBS','IBSCBSTribRegular','IBSCBSMono','IBSCBSTot',
        'gTransfCred','gCredPresIBSZFM','gAjusteCompet','gEstornoCred','gCredPresOper',
        'ICMSTot','ISSQNTot','total','Total',
        'transp','transporta','veicTransp','vol','lacres',
        'cobr','fat','dup','pag','detPag','infIntermed','infAdic','exporta','compra','infRespTec'
    ];

    public function certificate(array $input): array
    {
        $cert = $this->readCertificate($input);
        return ['cnpj'=>$cert->getCnpj(), 'fingerprint'=>openssl_x509_fingerprint((string)$cert->publicKey, 'sha256'), 'expires_at'=>$cert->getValidTo()->format(DATE_ATOM)];
    }

    private function readCertificate(array $input): Certificate
    {
        $raw = base64_decode($input['pfx_base64'] ?? '', true);
        if ($raw === false || strlen($raw) > 1500000) { throw new \DomainException('INVALID_CERTIFICATE'); }
        try { $cert = Certificate::readPfx($raw, $input['password'] ?? ''); }
        catch (\Throwable $e) { throw new \DomainException('INVALID_CERTIFICATE'); }
        if ($cert->getValidFrom()->getTimestamp() > time() || $cert->getValidTo()->getTimestamp() <= time()) { throw new \DomainException('CERTIFICATE_EXPIRED'); }
        if (!preg_match('/^[A-Z0-9]{12}[0-9]{2}$/', (string)$cert->getCnpj())) { throw new \DomainException('CERTIFICATE_CNPJ_REQUIRED'); }
        if (!openssl_x509_check_private_key((string)$cert->publicKey, (string)$cert->privateKey)) { throw new \DomainException('CERTIFICATE_KEY_MISMATCH'); }
        if (getenv('ALLOW_TEST_CERTIFICATES') !== 'true') {
            $ca = getenv('ICP_CA_FILE') ?: '/run/secrets/icp_ca';
            if (!is_file($ca) || filesize($ca) < 100) { throw new \DomainException('ICP_TRUST_STORE_REQUIRED'); }
            // Intermediates supplied inside the PKCS#12 are untrusted chain material.
            $bag = [];
            if (!openssl_pkcs12_read($raw, $bag, $input['password'] ?? '')) { throw new \DomainException('INVALID_CERTIFICATE'); }
            $intermediates = [];
            foreach ($bag['extracerts'] ?? [] as $c) {
                $p = openssl_x509_parse($c);
                if ($p && ($p['name'] ?? '') !== ($p['issuer']['name'] ?? '')) {
                    $intermediates[] = $c;
                }
            }
            $chain = tempnam(sys_get_temp_dir(), 'chain-');
            try {
                file_put_contents($chain, implode("\n", $intermediates));
                $valid = openssl_x509_checkpurpose((string)$cert->publicKey, X509_PURPOSE_ANY, [$ca], empty($intermediates) ? null : $chain);
                if ($valid !== true && $valid !== 1) { throw new \DomainException('CERTIFICATE_UNTRUSTED'); }
            } finally { if (is_file($chain)) { unlink($chain); } }
        }
        return $cert;
    }

    private function resolveTpAmb(array $input): int
    {
        $env = $input['document']['environment'] ?? ($input['environment'] ?? 'homologation');
        if ($env === 'production') {
            if (getenv('ENABLE_PRODUCTION') !== 'true' || getenv('ALLOW_TEST_CERTIFICATES') === 'true') {
                throw new \DomainException('ENVIRONMENT_NOT_ALLOWED');
            }
            return 1;
        }
        if ($env !== 'homologation') {
            throw new \DomainException('ENVIRONMENT_NOT_ALLOWED');
        }
        return 2;
    }

    private function tools(array $input): Tools
    {
        $tpAmb = $this->resolveTpAmb($input);
        $cert = $this->readCertificate($input['certificate']);
        $c = $input['company'];
        if ($cert->getCnpj() !== $c['cnpj']) { throw new \DomainException('CERTIFICATE_COMPANY_MISMATCH'); }
        $kind = $input['document']['kind'] ?? ($input['kind'] ?? 'nfe');
        $config = [
            'atualizacao'=>date('Y-m-d H:i:s'),
            'tpAmb'=>$tpAmb,
            'razaosocial'=>$c['name'],
            'cnpj'=>$c['cnpj'],
            'siglaUF'=>$c['uf'],
            'schemes'=>getenv('NFE_SCHEMA') ?: 'PL_010_V1.30',
            'versao'=>'4.00',
            'tokenIBPT'=>'',
            'CSC'=>$input['certificate']['csc'] ?? '',
            'CSCid'=>$input['certificate']['csc_id'] ?? ''
        ];
        if ($kind === 'nfce' && (!$config['CSC'] || !$config['CSCid'])) { throw new \DomainException('NFCE_CSC_REQUIRED'); }
        $tools = new Tools(json_encode($config, JSON_THROW_ON_ERROR), $cert);
        $tools->model($kind === 'nfce' ? 65 : 55);
        return $tools;
    }

    public function prepare(array $input): array
    {
        $tpAmb = $this->resolveTpAmb($input);
        $tools = $this->tools($input);
        $schema = getenv('NFE_SCHEMA') ?: 'PL_010_V1.30';
        $make = new Make($schema);
        $make->taginfNFe((object)['versao'=>'4.00']);
        $company = $input['company'];
        $kind = $input['document']['kind'] ?? 'nfe';
        $hasIde = false;
        $hasAutXML = false;
        foreach ($input['document']['invoice'] as $row) {
            $tag = $row['tag'] ?? '';
            if ($tag === 'autXML') { $hasAutXML = true; }
            if (!in_array($tag, self::TAGS, true) || !method_exists($make, 'tag'.$tag)) { throw new \DomainException('UNSUPPORTED_FISCAL_TAG'); }
            $v = $row['values'];
            if ($tag === 'emit') { continue; } // Issuer always comes from the authorized company.
            if ($tag === 'ide') {
                if ($hasIde) { throw new \DomainException('DUPLICATE_IDE'); }
                $hasIde = true;
                $v = array_merge($v, [
                    'cUF'=>self::UF[$company['uf']],
                    'mod'=>$kind==='nfce'?65:55,
                    'serie'=>$input['document']['series'],
                    'nNF'=>$input['number'],
                    'tpAmb'=>$tpAmb,
                    'tpEmis'=>1,
                    'dhEmi'=>(new \DateTimeImmutable($input['issued_at']))->setTimezone(new \DateTimeZone(\NFePHP\Common\TimeZoneByUF::get(self::UF[$company['uf']])))->format('Y-m-d\TH:i:sP'),
                    'procEmi'=>0,
                    'verProc'=>'AllSpark-0.3',
                    'cNF'=>substr(sprintf('%u', crc32($input['document_id'])), -8)
                ]);
            }
            if ($tag === 'enderEmit') { $v['UF'] = $company['uf']; }
            $make->{'tag'.$tag}((object)$v);
        }
        if (!$hasIde) { throw new \DomainException('IDE_REQUIRED'); }
        if (!$hasAutXML && ($company['uf'] ?? '') === 'BA') {
            $make->tagautXML((object)['CNPJ' => '13937073000156']);
        }
        $make->tagemit((object)['CNPJ'=>$company['cnpj'], 'xNome'=>$company['name'], 'IE'=>$company['ie'], 'CRT'=>$company['crt']]);
        try {
            $xml = $make->getXML();
            $signed = $tools->signNFe($xml); // Signing includes XSD validation from NFePHP.
        } catch (\Throwable $e) { throw new \DomainException('FISCAL_VALIDATION_FAILED'); }
        $dom = $this->xml($signed);
        $key = substr($dom->getElementsByTagName('infNFe')->item(0)->getAttribute('Id'), 3);
        $artifacts = [$this->artifact('signed_xml', 'application/xml', $signed)];
        $errors = [];
        try { $artifacts[] = $this->artifact('preview_pdf', 'application/pdf', $this->pdf($signed, $kind, (string)($company['logo_base64'] ?? ''))); }
        catch (\Throwable $e) { $errors[] = ['code'=>'PREVIEW_PDF_UNAVAILABLE', 'retryable'=>true]; }
        return ['status'=>'validated', 'access_key'=>$key, 'artifacts'=>$artifacts, 'protocol'=>null, 'errors'=>$errors];
    }

    public function transmit(array $input): array
    {
        if (getenv('ENABLE_ISSUANCE') !== 'true') { throw new \DomainException('TRANSMISSION_DISABLED'); }
        $tools = $this->tools($input);
        $signed = $input['signed_xml'];
        $this->assertIssuer($signed, $input);
        $batch = (string)($input['number']);
        $response = $tools->sefazEnviaLote([$signed], $batch, 1);
        return $this->result($signed, $response, $input['document']['kind'], (string)($input['company']['logo_base64'] ?? ''));
    }

    public function reconcile(array $input): array
    {
        if (getenv('ENABLE_ISSUANCE') !== 'true') { throw new \DomainException('TRANSMISSION_DISABLED'); }
        $tpAmb = $this->resolveTpAmb($input);
        $tools = $this->tools($input);
        $this->assertIssuer($input['signed_xml'], $input);
        $dom = $this->xml($input['signed_xml']);
        $key = substr($dom->getElementsByTagName('infNFe')->item(0)->getAttribute('Id'), 3);
        return $this->result($input['signed_xml'], $tools->sefazConsultaChave($key, $tpAmb), $input['document']['kind'], (string)($input['company']['logo_base64'] ?? ''));
    }

    public function status(array $input): array
    {
        if (getenv('ENABLE_ISSUANCE') !== 'true') { throw new \DomainException('TRANSMISSION_DISABLED'); }
        $tpAmb = $this->resolveTpAmb($input);
        $tools = $this->tools($input);
        $uf = $input['uf'] ?? ($input['company']['uf'] ?? '');
        $response = $tools->sefazStatus($uf, $tpAmb);
        $d = $this->xml($response);
        $code = $d->getElementsByTagName('cStat')->item(0)?->nodeValue ?? '';
        $reason = $d->getElementsByTagName('xMotivo')->item(0)?->nodeValue ?? '';
        $tMed = $d->getElementsByTagName('tMed')->item(0)?->nodeValue ?? '';
        return [
            'status' => $code === '107' ? 'online' : 'degraded',
            'uf' => $uf,
            'status_code' => $code,
            'reason' => $reason,
            'average_time_seconds' => $tMed !== '' ? (int)$tMed : null,
        ];
    }

    public function cancel(array $input): array
    {
        if (getenv('ENABLE_ISSUANCE') !== 'true') { throw new \DomainException('TRANSMISSION_DISABLED'); }
        $tools = $this->tools($input);
        $key = (string)($input['access_key'] ?? '');
        $reason = trim((string)($input['reason'] ?? ''));
        $protocol = (string)($input['protocol_number'] ?? '');
        if (!preg_match('/^[0-9A-Z]{44}$/', $key) || strlen($reason) < 15 || strlen($reason) > 255 || $protocol === '') {
            throw new \DomainException('INVALID_CANCEL_INPUT');
        }
        $response = $tools->sefazCancela($key, $reason, $protocol);
        $d = $this->xml($response);
        $xp = new \DOMXPath($d);
        $infEvento = $xp->query('//*[local-name()="infEvento"]')->item(0);
        $code = $infEvento ? ($infEvento->getElementsByTagName('cStat')->item(0)?->nodeValue ?? '') : ($d->getElementsByTagName('cStat')->item(0)?->nodeValue ?? '');
        $motivo = $infEvento ? ($infEvento->getElementsByTagName('xMotivo')->item(0)?->nodeValue ?? '') : ($d->getElementsByTagName('xMotivo')->item(0)?->nodeValue ?? '');
        $nProt = $infEvento ? ($infEvento->getElementsByTagName('nProt')->item(0)?->nodeValue ?? null) : null;
        $status = in_array($code, ['135','155','101'], true) ? 'canceled' : 'rejected';
        $artifacts = [$this->artifact('cancel_response_xml', 'application/xml', $response)];
        if ($status === 'canceled' && !empty($tools->lastRequest)) {
            try {
                $proc = Complements::toAuthorize($tools->lastRequest, $response);
                $artifacts[] = $this->artifact('cancel_xml', 'application/xml', $proc);
            } catch (\Throwable $e) {}
        }
        return [
            'status' => $status,
            'access_key' => $key,
            'artifacts' => $artifacts,
            'protocol' => ['status_code' => $code, 'number' => $nProt, 'reason' => $motivo],
            'errors' => $status === 'canceled' ? [] : [['code' => 'SEFAZ_CANCEL_REJECTED', 'retryable' => false]],
        ];
    }

    public function correction_letter(array $input): array
    {
        if (getenv('ENABLE_ISSUANCE') !== 'true') { throw new \DomainException('TRANSMISSION_DISABLED'); }
        $tools = $this->tools($input);
        $key = (string)($input['access_key'] ?? '');
        $correction = trim((string)($input['correction'] ?? ''));
        $seq = (int)($input['sequence'] ?? 1);
        if (!preg_match('/^[0-9A-Z]{44}$/', $key) || strlen($correction) < 15 || strlen($correction) > 1000 || $seq < 1 || $seq > 20) {
            throw new \DomainException('INVALID_CCE_INPUT');
        }
        $response = $tools->sefazCCe($key, $correction, $seq);
        $d = $this->xml($response);
        $xp = new \DOMXPath($d);
        $infEvento = $xp->query('//*[local-name()="infEvento"]')->item(0);
        $code = $infEvento ? ($infEvento->getElementsByTagName('cStat')->item(0)?->nodeValue ?? '') : ($d->getElementsByTagName('cStat')->item(0)?->nodeValue ?? '');
        $motivo = $infEvento ? ($infEvento->getElementsByTagName('xMotivo')->item(0)?->nodeValue ?? '') : ($d->getElementsByTagName('xMotivo')->item(0)?->nodeValue ?? '');
        $nProt = $infEvento ? ($infEvento->getElementsByTagName('nProt')->item(0)?->nodeValue ?? null) : null;
        $status = in_array($code, ['135','136'], true) ? 'registered' : 'rejected';
        $artifacts = [$this->artifact('cce_response_xml', 'application/xml', $response)];
        if ($status === 'registered' && !empty($tools->lastRequest)) {
            try {
                $proc = Complements::toAuthorize($tools->lastRequest, $response);
                $artifacts[] = $this->artifact('cce_xml', 'application/xml', $proc);
                if (class_exists(Dacce::class)) {
                    $c = $input['company'];
                    $aEnd = ['logradouro'=>$c['address']['street']??'','numero'=>$c['address']['number']??'','bairro'=>$c['address']['district']??'','cep'=>$c['address']['postal_code']??'','municipio'=>$c['address']['city']??'','UF'=>$c['uf']??'','telefone'=>'','email'=>''];
                    $artifacts[] = $this->artifact('cce_pdf', 'application/pdf', (new Dacce($proc, $aEnd))->render());
                }
            } catch (\Throwable $e) {}
        }
        return [
            'status' => $status,
            'access_key' => $key,
            'artifacts' => $artifacts,
            'protocol' => ['status_code' => $code, 'number' => $nProt, 'reason' => $motivo],
            'errors' => $status === 'registered' ? [] : [['code' => 'SEFAZ_CCE_REJECTED', 'retryable' => false]],
        ];
    }

    public function void_range(array $input): array
    {
        if (getenv('ENABLE_ISSUANCE') !== 'true') { throw new \DomainException('TRANSMISSION_DISABLED'); }
        $tpAmb = $this->resolveTpAmb($input);
        $tools = $this->tools($input);
        $series = (int)($input['series'] ?? -1);
        $start = (int)($input['start_number'] ?? 0);
        $end = (int)($input['end_number'] ?? 0);
        $reason = trim((string)($input['reason'] ?? ''));
        $year = (string)($input['year'] ?? date('y'));
        if ($series < 0 || $series > 889 || $start < 1 || $end < $start || strlen($reason) < 15 || strlen($reason) > 255) {
            throw new \DomainException('INVALID_VOID_INPUT');
        }
        $response = $tools->sefazInutiliza($series, $start, $end, $reason, $tpAmb, $year);
        $d = $this->xml($response);
        $xp = new \DOMXPath($d);
        $infInut = $xp->query('//*[local-name()="infInut"]')->item(0);
        $code = $infInut ? ($infInut->getElementsByTagName('cStat')->item(0)?->nodeValue ?? '') : ($d->getElementsByTagName('cStat')->item(0)?->nodeValue ?? '');
        $motivo = $infInut ? ($infInut->getElementsByTagName('xMotivo')->item(0)?->nodeValue ?? '') : ($d->getElementsByTagName('xMotivo')->item(0)?->nodeValue ?? '');
        $nProt = $infInut ? ($infInut->getElementsByTagName('nProt')->item(0)?->nodeValue ?? null) : null;
        $status = $code === '102' ? 'voided' : 'rejected';
        $artifacts = [$this->artifact('void_response_xml', 'application/xml', $response)];
        if ($status === 'voided' && !empty($tools->lastRequest)) {
            try {
                $proc = Complements::toAuthorize($tools->lastRequest, $response);
                $artifacts[] = $this->artifact('void_xml', 'application/xml', $proc);
            } catch (\Throwable $e) {}
        }
        return [
            'status' => $status,
            'artifacts' => $artifacts,
            'protocol' => ['status_code' => $code, 'number' => $nProt, 'reason' => $motivo],
            'errors' => $status === 'voided' ? [] : [['code' => 'SEFAZ_VOID_REJECTED', 'retryable' => false]],
        ];
    }

    public function dist_dfe(array $input): array
    {
        if (getenv('ENABLE_ISSUANCE') !== 'true') { throw new \DomainException('TRANSMISSION_DISABLED'); }
        $tools = $this->tools($input);
        $ultNSU = (int)($input['last_nsu'] ?? 0);
        $numNSU = (int)($input['nsu'] ?? 0);
        $chave = isset($input['access_key']) && $input['access_key'] !== '' ? (string)$input['access_key'] : null;
        $response = $tools->sefazDistDFe($ultNSU, $numNSU, $chave);
        $d = $this->xml($response);
        $code = $d->getElementsByTagName('cStat')->item(0)?->nodeValue ?? '';
        $motivo = $d->getElementsByTagName('xMotivo')->item(0)?->nodeValue ?? '';
        $nextNSU = $d->getElementsByTagName('ultNSU')->item(0)?->nodeValue ?? '000000000000000';
        $maxNSU = $d->getElementsByTagName('maxNSU')->item(0)?->nodeValue ?? '000000000000000';
        $documents = [];
        foreach ($d->getElementsByTagName('docZip') as $node) {
            $nsu = $node->getAttribute('NSU');
            $schema = $node->getAttribute('schema');
            $decoded = @gzdecode((string)base64_decode($node->nodeValue, true));
            if ($decoded !== false) {
                $documents[] = ['nsu' => $nsu, 'schema' => $schema, 'xml_base64' => base64_encode($decoded)];
            }
        }
        return [
            'status_code' => $code,
            'reason' => $motivo,
            'last_nsu' => $nextNSU,
            'max_nsu' => $maxNSU,
            'documents' => $documents,
        ];
    }

    public function manifest(array $input): array
    {
        if (getenv('ENABLE_ISSUANCE') !== 'true') { throw new \DomainException('TRANSMISSION_DISABLED'); }
        $tools = $this->tools($input);
        $key = (string)($input['access_key'] ?? '');
        $type = (string)($input['type'] ?? 'ciencia');
        $reason = trim((string)($input['reason'] ?? ''));
        $seq = (int)($input['sequence'] ?? 1);
        $map = [
            'ciencia' => 210210,
            'confirmacao' => 210200,
            'desconhecimento' => 210220,
            'nao_realizada' => 210240,
        ];
        if (!preg_match('/^[0-9A-Z]{44}$/', $key) || !isset($map[$type])) {
            throw new \DomainException('INVALID_MANIFEST_INPUT');
        }
        if ($type === 'nao_realizada' && (strlen($reason) < 15 || strlen($reason) > 255)) {
            throw new \DomainException('INVALID_MANIFEST_INPUT');
        }
        $response = $tools->sefazManifesta($key, $map[$type], $reason, $seq);
        $d = $this->xml($response);
        $xp = new \DOMXPath($d);
        $infEvento = $xp->query('//*[local-name()="infEvento"]')->item(0);
        $code = $infEvento ? ($infEvento->getElementsByTagName('cStat')->item(0)?->nodeValue ?? '') : ($d->getElementsByTagName('cStat')->item(0)?->nodeValue ?? '');
        $motivo = $infEvento ? ($infEvento->getElementsByTagName('xMotivo')->item(0)?->nodeValue ?? '') : ($d->getElementsByTagName('xMotivo')->item(0)?->nodeValue ?? '');
        $nProt = $infEvento ? ($infEvento->getElementsByTagName('nProt')->item(0)?->nodeValue ?? null) : null;
        $status = in_array($code, ['135','136'], true) ? 'registered' : 'rejected';
        return [
            'status' => $status,
            'access_key' => $key,
            'type' => $type,
            'protocol' => ['status_code' => $code, 'number' => $nProt, 'reason' => $motivo],
            'errors' => $status === 'registered' ? [] : [['code' => $code !== '' ? 'SEFAZ_'.$code : 'SEFAZ_MANIFEST_REJECTED', 'retryable' => false]],
        ];
    }

    private function assertIssuer(string $xml, array $input): void
    {
        $tpAmb = (string)$this->resolveTpAmb($input);
        $d = $this->xml($xml);
        $xp = new \DOMXPath($d); $xp->registerNamespace('n','http://www.portalfiscal.inf.br/nfe');
        foreach (['//n:emit/n:CNPJ'=>$input['company']['cnpj'], '//n:ide/n:tpAmb'=>$tpAmb, '//n:ide/n:mod'=>($input['document']['kind']??'nfe')==='nfce'?'65':'55'] as $path=>$expected) {
            if ($xp->evaluate('string('.$path.')') !== $expected) { throw new \DomainException('XML_CONTEXT_MISMATCH'); }
        }
    }

    private function result(string $signed, string $response, string $kind, string $logo = ''): array
    {
        $d = $this->xml($response); $xp = new \DOMXPath($d);
        $protocol = $xp->query('//*[local-name()="infProt"]')->item(0);
        $codes = $d->getElementsByTagName('cStat');
        $code = $protocol ? $protocol->getElementsByTagName('cStat')->item(0)->nodeValue : ($codes->length ? $codes->item(0)->nodeValue : '');
        $status = 'reconciliation_required';
        if (in_array($code, ['100','150'], true) && $protocol) { $status = 'authorized'; }
        elseif (in_array($code, ['101','151','155'], true) && $protocol) { $status = 'canceled'; }
        elseif ($protocol && !in_array($code, ['204','539'], true)) { $status = 'rejected'; }
        // Batch-level/duplicate/unknown responses remain uncertain; never blindly resend.
        $artifacts = [$this->artifact('protocol_xml','application/xml',$response)];
        $errors = [];
        if ($status === 'rejected' && $code !== '') {
            $errors[] = ['code' => 'SEFAZ_'.$code, 'retryable' => false];
        } elseif ($code === '656') {
            $errors[] = ['code' => 'SEFAZ_656', 'retryable' => false];
        }
        if ($status === 'authorized') {
            try { $processed = Complements::toAuthorize($signed, $response); }
            catch (\Throwable $e) { return ['status'=>'reconciliation_required','artifacts'=>$artifacts,'errors'=>[['code'=>'PROTOCOL_ASSEMBLY_FAILED','retryable'=>true]]]; }
            $artifacts[] = $this->artifact('authorized_xml','application/xml',$processed);
            try { $artifacts[] = $this->artifact('danfe_pdf','application/pdf',$this->pdf($processed,$kind,$logo)); }
            catch (\Throwable $e) { $errors[] = ['code'=>'PDF_UNAVAILABLE','retryable'=>true]; }
        }
        return ['status'=>$status, 'artifacts'=>$artifacts, 'protocol'=>['status_code'=>$code, 'number'=>$protocol?($protocol->getElementsByTagName('nProt')->item(0)?->nodeValue):null, 'reason'=>$protocol?($protocol->getElementsByTagName('xMotivo')->item(0)?->nodeValue):null], 'errors'=>$errors];
    }
    private function pdf(string $xml, string $kind, string $logo = ''): string {
        $logoUri = $logo !== '' ? 'data://text/plain;base64,'.$logo : '';
        return $kind === 'nfce' ? (new Danfce($xml))->render($logoUri) : (new Danfe($xml))->render($logoUri);
    }
    private function artifact(string $kind, string $type, string $bytes): array { return ['kind'=>$kind,'media_type'=>$type,'base64'=>base64_encode($bytes)]; }
    private function xml(string $xml): \DOMDocument {
        if (stripos($xml,'<!DOCTYPE') !== false || stripos($xml,'<!ENTITY') !== false) { throw new \DomainException('UNSAFE_XML'); }
        $d = new \DOMDocument();
        if (!$d->loadXML($xml, LIBXML_NONET)) { throw new \DomainException('INVALID_XML'); }
        return $d;
    }
}
