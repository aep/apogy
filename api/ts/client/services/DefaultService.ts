/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { Document } from '../models/Document';
import type { PutDocumentOK } from '../models/PutDocumentOK';
import type { ReactorIn } from '../models/ReactorIn';
import type { ReactorOut } from '../models/ReactorOut';
import type { SearchRequest } from '../models/SearchRequest';
import type { SearchResponse } from '../models/SearchResponse';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class DefaultService {
    /**
     * Get a document by model and ID
     * @param model
     * @param id
     * @returns Document Successfully retrieved document
     * @throws ApiError
     */
    public static getDocument(
        model: string,
        id: string,
    ): CancelablePromise<Document> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/v1/{model}/{id}',
            path: {
                'model': model,
                'id': id,
            },
        });
    }
    /**
     * Delete a document by model and ID
     * @param model
     * @param id
     * @returns any Successfully deleted document
     * @throws ApiError
     */
    public static deleteDocument(
        model: string,
        id: string,
    ): CancelablePromise<any> {
        return __request(OpenAPI, {
            method: 'DELETE',
            url: '/v1/{model}/{id}',
            path: {
                'model': model,
                'id': id,
            },
            errors: {
                400: `Validation Error`,
                404: `Document not found`,
            },
        });
    }
    /**
     * Create or update a document
     * @param requestBody
     * @returns PutDocumentOK Successfully stored document
     * @throws ApiError
     */
    public static putDocument(
        requestBody: Document,
    ): CancelablePromise<PutDocumentOK> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/v1',
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `Validation Error`,
                409: `Conflict`,
            },
        });
    }
    /**
     * Search for documents
     * @param requestBody
     * @returns SearchResponse Search results
     * @throws ApiError
     */
    public static searchDocuments(
        requestBody: SearchRequest,
    ): CancelablePromise<SearchResponse> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/v1/q',
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `Validation Error`,
            },
        });
    }
    /**
     * Bidirectional streaming for reactor operations
     * @param requestBody
     * @returns ReactorOut Successful connection
     * @throws ApiError
     */
    public static reactorLoop(
        requestBody: ReactorIn,
    ): CancelablePromise<ReactorOut> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/v1/reactor',
            body: requestBody,
            mediaType: 'application/json',
        });
    }
}
