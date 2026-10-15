<?php
declare(strict_types=1);

// Fail-closed acceptance test: disabled paper-marker publication must not
// open an upstream connection or emit execution metadata.
$entry = dirname(__DIR__, 3) . '/apps/workspace/public/api/v1/paper-markers/index.php';
if (!is_file($entry)) {
    fwrite(STDERR, "missing public paper marker route\n");
    exit(1);
}
$command = escapeshellarg(PHP_BINARY) . ' ' . escapeshellarg($entry);
$descriptor = [1 => ['pipe', 'w'], 2 => ['pipe', 'w']];
$process = proc_open($command, $descriptor, $pipes, null, [
    'QNEXT_PUBLIC_PAPER_MARKERS' => '0',
    'QNEXT_SYN_PLUS_PAPER_URL' => 'http://127.0.0.1:1',
]);
if (!is_resource($process)) {
    fwrite(STDERR, "could not launch marker visibility check\n");
    exit(1);
}
$output = stream_get_contents($pipes[1]);
$error = stream_get_contents($pipes[2]);
fclose($pipes[1]); fclose($pipes[2]);
$status = proc_close($process);
$data = json_decode(is_string($output) ? $output : '', true);
if ($status !== 0 || !is_array($data) || ($data['available'] ?? null) !== false
    || ($data['enabled'] ?? null) !== false || ($data['markers'] ?? null) !== []
    || ($data['sessionId'] ?? null) !== '') {
    fwrite(STDERR, 'private paper markers leaked: ' . (string) $error . PHP_EOL);
    exit(1);
}
echo "Private paper-marker publication default: PASS\n";
