'use strict';

// Load modules

const Hoek = require('hoek');


// Declare internals

const internals = {
    codes: new Map([
        [100, 'Continue'],
        [101, 'Switching Protocols'],
        [102, 'Processing'],
        [200, 'OK'],
        [201, 'Created'],
        [202, 'Accepted'],
        [203, 'Non-Authoritative Information'],
        [204, 'No Content'],
        [205, 'Reset Content'],
        [206, 'Partial Content'],
        [207, 'Multi-Status'],
        [300, 'Multiple Choices'],
        [301, 'Moved Permanently'],
        [302, 'Moved Temporarily'],
        [303, 'See Other'],
        [304, 'Not Modified'],
        [305, 'Use Proxy'],
        [307, 'Temporary Redirect'],
        [400, 'Bad Request'],
        [401, 'Unauthorized'],
        [402, 'Payment Required'],
        [403, 'Forbidden'],
        [404, 'Not Found'],
        [405, 'Method Not Allowed'],
        [406, 'Not Acceptable'],
        [407, 'Proxy Authentication Required'],
        [408, 'Request Time-out'],
        [409, 'Conflict'],
        [410, 'Gone'],
        [411, 'Length Required'],
        [412, 'Precondition Failed'],
        [413, 'Request Entity Too Large'],
        [414, 'Request-URI Too Large'],
        [415, 'Unsupported Media Type'],
        [416, 'Requested Range Not Satisfiable'],
        [417, 'Expectation Failed'],
        [418, 'I\'m a teapot'],
        [422, 'Unprocessable Entity'],
        [423, 'Locked'],
        [424, 'Failed Dependency'],
        [425, 'Unordered Collection'],
        [426, 'Upgrade Required'],
        [428, 'Precondition Required'],
        [429, 'Too Many Requests'],
        [431, 'Request Header Fields Too Large'],
        [451, 'Unavailable For Legal Reasons'],
        [500, 'Internal Server Error'],
        [501, 'Not Implemented'],
        [502, 'Bad Gateway'],
        [503, 'Service Unavailable'],
        [504, 'Gateway Time-out'],
        [505, 'HTTP Version Not Supported'],
        [506, 'Variant Also Negotiates'],
        [507, 'Insufficient Storage'],
        [509, 'Bandwidth Limit Exceeded'],
        [510, 'Not Extended'],
        [511, 'Network Authentication Required']
    ])
};


module.exports = internals.Boom = class extends Error {

    static [Symbol.hasInstance](instance) {

        return internals.Boom.isBoom(instance);
    }

    constructor(message, options = {}) {

        if (message instanceof Error) {
            return internals.Boom.boomify(Hoek.clone(message), options);
        }

        const { statusCode = 500, data = null, ctor = internals.Boom } = options;
        const error = new Error(message ? message : undefined);         // Avoids settings null message
        Error.captureStackTrace(error, ctor);                           // Filter the stack to our external API
        error.data = data;
        internals.initialize(error, statusCode);
        error.typeof = ctor;

        if (options.decorate) {
            Object.assign(error, options.decorate);
        }

        return error;
    }

    static isBoom(err) {

        return (err instanceof Error && !!err.isBoom);
    }

    static boomify(err, options) {

        Hoek.assert(err instanceof Error, 'Cannot wrap non-Error object');

        options = options || {};

        if (options.data !== undefined) {
            err.data = options.data;
        }

        if (options.decorate) {
            Object.assign(err, options.decorate);
        }

        if (!err.isBoom) {
            return internals.initialize(err, options.statusCode || 500, options.message);
        }

        if (options.override === false ||                           // Defaults to true
            (!options.statusCode && !options.message)) {

            return err;
        }

        return internals.initialize(err, options.statusCode || err.output.statusCode, options.message);
    }

    // 4xx Client Errors

    static badRequest(message, data) {

        return new internals.Boom(message, { statusCode: 400, data, ctor: internals.Boom.badRequest });
    }

    static unauthorized(message, scheme, attributes) {          // Or function (message, wwwAuthenticate[])

        const err = new internals.Boom(message, { statusCode: 401, ctor: internals.Boom.unauthorized });

        if (!scheme) {
            return err;
        }

        let wwwAuthenticate = '';

        if (typeof scheme === 'string') {

            // function (message, scheme, attributes)

            wwwAuthenticate = scheme;

            if (attributes || message) {
                err.output.payload.attributes = {};
            }

            if (attributes) {
                if (typeof attributes === 'string') {
                    wwwAuthenticate = wwwAuthenticate + ' ' + Hoek.escapeHeaderAttribute(attributes);
                    err.output.payload.attributes = attributes;
                }
                else {
                    const names = Object.keys(attributes);
                    for (let i = 0; i < names.length; ++i) {
                        const name = names[i];
                        if (i) {
                            wwwAuthenticate = wwwAuthenticate + ',';
                        }

                        let value = attributes[name];
                        if (value === null ||
                            value === undefined) {              // Value can be zero

                            value = '';
                        }

                        wwwAuthenticate = wwwAuthenticate + ' ' + name + '="' + Hoek.escapeHeaderAttribute(value.toString()) + '"';
                        err.output.payload.attributes[name] = value;
                    }
                }
            }


            if (message) {
                if (attributes) {
                    wwwAuthenticate = wwwAuthenticate + ',';
                }

                wwwAuthenticate = wwwAuthenticate + ' error="' + Hoek.escapeHeaderAttribute(message) + '"';
                err.output.payload.attributes.error = message;
            }
            else {
                err.isMissing = true;
            }
        }
        else {

            // function (message, wwwAuthenticate[])

            const wwwArray = scheme;
            for (let i = 0; i < wwwArray.length; ++i) {
                if (i) {
                    wwwAuthenticate = wwwAuthenticate + ', ';
                }

                wwwAuthenticate = wwwAuthenticate + wwwArray[i];
            }
        }

        err.output.headers['WWW-Authenticate'] = wwwAuthenticate;

        return err;
    }

    static paymentRequired(message, data) {

        return new internals.Boom(message, { statusCode: 402, data, ctor: internals.Boom.paymentRequired });
    }

    static forbidden(message, data) {

        return new internals.Boom(message, { statusCode: 403, data, ctor: internals.Boom.forbidden });
    }

    static notFound(message, data) {

        return new internals.Boom(message, { statusCode: 404, data, ctor: internals.Boom.notFound });
    }

    static methodNotAllowed(message, data, allow) {

        const err = new internals.Boom(message, { statusCode: 405, data, ctor: internals.Boom.methodNotAllowed });

        if (typeof allow === 'string') {
            allow = [allow];
        }

        if (Array.isArray(allow)) {
            err.output.headers.Allow = allow.join(', ');
        }

        return err;
    }

    static notAcceptable(message, data) {

        return new internals.Boom(message, { statusCode: 406, data, ctor: internals.Boom.notAcceptable });
    }

    static proxyAuthRequired(message, data) {

        return new internals.Boom(message, { statusCode: 407, data, ctor: internals.Boom.proxyAuthRequired });
    }

    static clientTimeout(message, data) {

        return new internals.Boom(message, { statusCode: 408, data, ctor: internals.Boom.clientTimeout });
    }

    static conflict(message, data) {

        return new internals.Boom(message, { statusCode: 409, data, ctor: internals.Boom.conflict });
    }

    static resourceGone(message, data) {

        return new internals.Boom(message, { statusCode: 410, data, ctor: internals.Boom.resourceGone });
    }

    static lengthRequired(message, data) {

        return new internals.Boom(message, { statusCode: 411, data, ctor: internals.Boom.lengthRequired });
    }

    static preconditionFailed(message, data) {

        return new internals.Boom(message, { statusCode: 412, data, ctor: internals.Boom.preconditionFailed });
    }

    static entityTooLarge(message, data) {

        return new internals.Boom(message, { statusCode: 413, data, ctor: internals.Boom.entityTooLarge });
    }

    static uriTooLong(message, data) {

        return new internals.Boom(message, { statusCode: 414, data, ctor: internals.Boom.uriTooLong });
    }

    static unsupportedMediaType(message, data) {

        return new internals.Boom(message, { statusCode: 415, data, ctor: internals.Boom.unsupportedMediaType });
    }

    static rangeNotSatisfiable(message, data) {

        return new internals.Boom(message, { statusCode: 416, data, ctor: internals.Boom.rangeNotSatisfiable });
    }

    static expectationFailed(message, data) {

        return new internals.Boom(message, { statusCode: 417, data, ctor: internals.Boom.expectationFailed });
    }

    static teapot(message, data) {

        return new internals.Boom(message, { statusCode: 418, data, ctor: internals.Boom.teapot });
    }

    static badData(message, data) {

        return new internals.Boom(message, { statusCode: 422, data, ctor: internals.Boom.badData });
    }

    static locked(message, data) {

        return new internals.Boom(message, { statusCode: 423, data, ctor: internals.Boom.locked });
    }

    static failedDependency(message, data) {

        return new internals.Boom(message, { statusCode: 424, data, ctor: internals.Boom.failedDependency });
    }

    static preconditionRequired(message, data) {

        return new internals.Boom(message, { statusCode: 428, data, ctor: internals.Boom.preconditionRequired });
    }

    static tooManyRequests(message, data) {

        return new internals.Boom(message, { statusCode: 429, data, ctor: internals.Boom.tooManyRequests });
    }

    static illegal(message, data) {

        return new internals.Boom(message, { statusCode: 451, data, ctor: internals.Boom.illegal });
    }

    // 5xx Server Errors

    static internal(message, data, statusCode = 500) {

        return internals.serverError(message, data, statusCode, internals.Boom.internal);
    }

    static notImplemented(message, data) {

        return internals.serverError(message, data, 501, internals.Boom.notImplemented);
    }

    static badGateway(message, data) {

        return internals.serverError(message, data, 502, internals.Boom.badGateway);
    }

    static serverUnavailable(message, data) {

        return internals.serverError(message, data, 503, internals.Boom.serverUnavailable);
    }

    static gatewayTimeout(message, data) {

        return internals.serverError(message, data, 504, internals.Boom.gatewayTimeout);
    }

    static badImplementation(message, data) {

        const err = internals.serverError(message, data, 500, internals.Boom.badImplementation);
        err.isDeveloperError = true;
        return err;
    }
};



internals.initialize = function (err, statusCode, message) {

    const numberCode = parseInt(statusCode, 10);
    Hoek.assert(!isNaN(numberCode) && numberCode >= 400, 'First argument must be a number (400+):', statusCode);

    err.isBoom = true;
    err.isServer = numberCode >= 500;

    if (!err.hasOwnProperty('data')) {
        err.data = null;
    }

    err.output = {
        statusCode: numberCode,
        payload: {},
        headers: {}
    };

    err.reformat = internals.reformat;

    if (!message &&
        !err.message) {

        err.reformat();
        message = err.output.payload.error;
    }

    if (message) {
        err.message = (message + (err.message ? ': ' + err.message : ''));
        err.output.payload.message = err.message;
    }

    err.reformat();
    return err;
};


internals.reformat = function (debug = false) {

    this.output.payload.statusCode = this.output.statusCode;
    this.output.payload.error = internals.codes.get(this.output.statusCode) || 'Unknown';

    if (this.output.statusCode === 500 && debug !== true) {
        this.output.payload.message = 'An internal server error occurred';              // Hide actual error from user
    }
    else if (this.message) {
        this.output.payload.message = this.message;
    }
};


internals.serverError = function (message, data, statusCode, ctor) {

    if (data instanceof Error &&
        !data.isBoom) {

        return internals.Boom.boomify(data, { statusCode, message });
    }

    return new internals.Boom(message, { statusCode, data, ctor });
};
