DROP INDEX console_observation_deployment;
CREATE INDEX console_observation_deployment ON console_observation(deployment_id, recorded_at DESC, id DESC);

DROP INDEX console_activity_scope;
CREATE INDEX console_activity_scope ON console_activity(connection_id, namespace, recorded_at DESC, id DESC);
