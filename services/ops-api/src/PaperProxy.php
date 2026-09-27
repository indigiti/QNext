<?php

declare(strict_types=1);

namespace QNext\Ops;

use JsonException;
use RuntimeException;

final class PaperProxy
{
    public function __construct(private readonly string $baseUrl)
    {
    }

    public function status(): array
    {
        $response = $this->request('/status', 'GET', null);
        if (!($response['ok'] ?? false)) {
            return [
                'ok' => false,
                'status' => $response['status'] ?? null,
                'error' => $response['error'] ?? 'SYN+ paper runtime unavailable',
            ];
        }

        return [
            'ok' => true,
            'status' => $response['status'] ?? 200,
            'body' => is_array($response['body'] ?? null) ? $response['body'] : [],
        ];
    }

    public function control(array $payload): array
    {
        $action = $payload['action'] ?? null;
        if (!is_string($action) || !in_array($action, ['enable', 'disable', 'reset', 'configure'], true)) {
            throw new RuntimeException('paper action must be enable, disable, reset, or configure');
        }

        $response = $this->request('/control', 'POST', $payload);
        if (!($response['ok'] ?? false)) {
            $body = $response['body'] ?? null;
            $message = is_array($body)
                ? (string) ($body['error'] ?? 'SYN+ paper control failed')
                : (string) ($response['error'] ?? 'SYN+ paper control failed');
            throw new RuntimeException($message);
        }

        return is_array($response['body'] ?? null) ? $response['body'] : [];
    }

    private function request(string $path, string $method, ?array $payload): array
    {
        $headers = "Accept: application/json\r\n";
        $content = null;
        if ($payload !== null) {
            try {
                $content = json_encode($payload, JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR);
            } catch (JsonException $error) {
                throw new RuntimeException('cannot encode paper runtime request', 0, $error);
            }
            $headers .= "Content-Type: application/json\r\n";
        }

        $http = [
            'method' => $method,
            'timeout' => 2.0,
            'ignore_errors' => true,
            'header' => $headers,
        ];
        if ($content !== null) {
            $http['content'] = $content;
        }

        $context = stream_context_create(['http' => $http]);
        $body = @file_get_contents(rtrim($this->baseUrl, '/') . $path, false, $context);
        $status = $this->responseStatus($http_response_header ?? []);
        if ($body === false) {
            return ['ok' => false, 'status' => $status ?: null, 'error' => 'connection failed'];
        }

        $decoded = json_decode($body, true);
        return [
            'ok' => $status >= 200 && $status < 300,
            'status' => $status,
            'body' => is_array($decoded) ? $decoded : $body,
        ];
    }

    private function responseStatus(array $headers): int
    {
        foreach ($headers as $header) {
            if (preg_match('/^HTTP\/\S+\s+(\d{3})/', $header, $matches)) {
                return (int) $matches[1];
            }
        }
        return 0;
    }
}
