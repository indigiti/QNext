<?php

declare(strict_types=1);

namespace QNext\Ops;

final class Auth
{
    public function __construct(private readonly string $expectedToken)
    {
    }

    public function authorized(?string $providedToken): bool
    {
        if ($this->expectedToken === '' || $providedToken === null || $providedToken === '') {
            return false;
        }

        return hash_equals($this->expectedToken, $providedToken);
    }
}
