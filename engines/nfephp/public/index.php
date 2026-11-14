<?php
declare(strict_types=1);
require __DIR__.'/../vendor/autoload.php';
header('Content-Type: application/json');
header('Cache-Control: no-store');
ini_set('display_errors','0');
libxml_use_internal_errors(true);
$path = parse_url($_SERVER['REQUEST_URI'], PHP_URL_PATH);
if ($path === '/healthz') { echo '{"status":"ok"}'; exit; }
$token = trim((string)file_get_contents(getenv('ENGINE_TOKEN_FILE') ?: '/run/secrets/engine_token'));
if (strlen($token) < 32 || !hash_equals('Bearer '.$token, $_SERVER['HTTP_AUTHORIZATION'] ?? '')) {
    http_response_code(401); echo '{"code":"UNAUTHORIZED","retryable":false}'; exit;
}
$action = substr($path,1);
$allowed = ['certificate','prepare','transmit','reconcile','status','cancel','correction_letter','void_range','dist_dfe','manifest'];
if ($_SERVER['REQUEST_METHOD'] !== 'POST' || !in_array($action, $allowed, true)) {
    http_response_code(404); echo '{"code":"NOT_FOUND","retryable":false}'; exit;
}
try {
    $raw = file_get_contents('php://input', false, null, 0, 8*1024*1024+1);
    if (strlen($raw) > 8*1024*1024) { throw new DomainException('PAYLOAD_TOO_LARGE'); }
    $input = json_decode($raw,true,64,JSON_THROW_ON_ERROR);
    if (!is_array($input)) { throw new DomainException('INVALID_INPUT'); }
    $engine = new AllSpark\Fiscal\Engine();
    echo json_encode($engine->$action($input),JSON_THROW_ON_ERROR);
} catch (DomainException $e) {
    http_response_code(422);
    echo json_encode(['code'=>preg_match('/^[A-Z][A-Z0-9_]{1,80}$/', $e->getMessage()) ? $e->getMessage() : 'FISCAL_VALIDATION_FAILED','retryable'=>false]);
} catch (Throwable $e) {
    // Do not serialize library exceptions: they may contain XML, PFX or passwords.
    http_response_code(503);
    echo '{"code":"ENGINE_FAILURE","retryable":true}';
}
