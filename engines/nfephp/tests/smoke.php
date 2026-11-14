<?php
// Runs in a disposable container with a synthetic certificate; no SEFAZ access.
require __DIR__.'/../vendor/autoload.php';
putenv('ALLOW_TEST_CERTIFICATES=true');
putenv('ENABLE_ISSUANCE=false');
$engine = new AllSpark\Fiscal\Engine();
$cert = ['pfx_base64'=>base64_encode(file_get_contents('/fixtures/test.pfx')), 'password'=>'synthetic-only'];
$info=$engine->certificate($cert);
if ($info['cnpj'] !== '11222333000181') { throw new RuntimeException('CNPJ extraction failed'); }
$input=['certificate'=>$cert,'company'=>json_decode(file_get_contents('/examples/company.json'),true),'document'=>json_decode(file_get_contents('/examples/document.json'),true),'number'=>1,'document_id'=>'a8be0e90-b70e-4e3c-848c-24fb51d41c78','issued_at'=>date('c')];
$input['document']['invoice']=json_decode(file_get_contents(__DIR__.'/invoice-tags.json'),true);
try { $result=$engine->prepare($input); }
catch(Throwable $e) { fwrite(STDERR,$e->getMessage()."\n");exit(1); }
if($result['status']!=='validated') { throw new RuntimeException('Generation failed'); }
$types=array_column($result['artifacts'],'kind');
if(!in_array('signed_xml',$types)||!in_array('preview_pdf',$types)) { throw new RuntimeException('XML/PDF missing: '.json_encode($result['errors'])); }
foreach($result['artifacts'] as $a) {
 $raw=base64_decode($a['base64']);
 if($a['kind']==='preview_pdf' && substr($raw,0,5)!=='%PDF-') {throw new RuntimeException('Invalid PDF');}
}
// Also test Reforma Tributaria (IBSCBS + IBSCBSTot) and DIFAL (ICMSUFDest) XML + XSD + PDF
$rtcTags = json_decode(file_get_contents(__DIR__.'/invoice-tags.json'), true);
$rtcTags[] = [
 'tag' => 'ICMSUFDest',
 'values' => [
  'item' => 1,
  'vBCUFDest' => '10.00',
  'vBCFCPUFDest' => '10.00',
  'pFCPUFDest' => '2.00',
  'pICMSUFDest' => '18.00',
  'pICMSInter' => '12.00',
  'pICMSInterPart' => '100.00',
  'vFCPUFDest' => '0.20',
  'vICMSUFDest' => '0.60',
  'vICMSUFRemet' => '0.00',
 ],
];
$rtcTags[] = [
 'tag' => 'IBSCBS',
 'values' => [
  'item' => 1,
  'CST' => '000',
  'cClassTrib' => '000001',
  'vBC' => '10.00',
  'gIBSUF_pIBSUF' => '0.10',
  'gIBSUF_vIBSUF' => '0.01',
  'gIBSMun_pIBSMun' => '0.00',
  'gIBSMun_vIBSMun' => '0.00',
  'gCBS_pCBS' => '0.90',
  'gCBS_vCBS' => '0.09',
  'vIBS' => '0.01',
 ],
];
$rtcTags[] = [
 'tag' => 'IBSCBSTot',
 'values' => [
  'vBCIBSCBS' => '10.00',
  'gIBS_vIBS' => '0.01',
  'gIBS_vCredPres' => '0.00',
  'gIBS_vCredPresCondSus' => '0.00',
  'gIBSUF_vDif' => '0.00',
  'gIBSUF_vDevTrib' => '0.00',
  'gIBSUF_vIBSUF' => '0.01',
  'gIBSMun_vDif' => '0.00',
  'gIBSMun_vDevTrib' => '0.00',
  'gIBSMun_vIBSMun' => '0.00',
  'gCBS_vDif' => '0.00',
  'gCBS_vDevTrib' => '0.00',
  'gCBS_vCBS' => '0.09',
  'gCBS_vCredPres' => '0.00',
  'gCBS_vCredPresCondSus' => '0.00',
 ],
];
$input['document']['invoice'] = $rtcTags;
$input['number'] = 2;
$rtcResult = $engine->prepare($input);
if ($rtcResult['status'] !== 'validated') {
 throw new RuntimeException('RTC + DIFAL generation failed: '.json_encode($rtcResult['errors']));
}
$rtcXml = '';
foreach ($rtcResult['artifacts'] as $a) {
 if ($a['kind'] === 'signed_xml') {
  $rtcXml = base64_decode($a['base64']);
 }
}
if (strpos($rtcXml, '<IBSCBS>') === false || strpos($rtcXml, '<IBSCBSTot>') === false || strpos($rtcXml, '<ICMSUFDest>') === false) {
 throw new RuntimeException('Missing IBSCBS/IBSCBSTot/ICMSUFDest tags in signed XML');
}
try{$engine->transmit($input);throw new RuntimeException('Transmission unexpectedly enabled');}catch(DomainException $e){if($e->getMessage()!=='TRANSMISSION_DISABLED'){throw $e;}}
putenv('ALLOW_TEST_CERTIFICATES=false');
try{$engine->certificate($cert);throw new RuntimeException('Untrusted cert accepted');}catch(DomainException $e){if(!in_array($e->getMessage(),['ICP_TRUST_STORE_REQUIRED','CERTIFICATE_UNTRUSTED'])){throw $e;}}
echo "PASS: certificate parsing, signed XML/XSD, PDF, transmission guard, trust guard\n";
